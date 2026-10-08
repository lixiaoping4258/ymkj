package biz

import (
	"context"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/http/middleware/WhitelistMiddleware.php
//   app/api/logic/whitelist/WhitelistLogic.php
//
// 白名单是一套「限制项」机制：把用户分到白名单组，组里逐项配置某类操作是否禁用。
// market 域有 8 条路由挂了它。

// 限制项 code，取自表 x_whitelist_item.code（status=1 的才算启用）。
const (
	WhitelistItemNoCollectionTrade = "no_collection_trade"
	WhitelistItemNoWithdraw        = "no_withdraw"
	WhitelistItemNoSync            = "no_sync"
	WhitelistItemNoTeaTrade        = "no_tea_trade"
	WhitelistItemNoTaoTrade        = "no_tao_trade"
)

// WhitelistCacheKey 与 PHP 的 Cache::get('whitelist:user_perm:'.$userId) 对应。
// 缓存 300 秒。
const whitelistCacheTTL = 300 * time.Second

// WhitelistRepo 白名单数据访问。
type WhitelistRepo interface {
	// EnabledItems 对应 WhitelistItem::where('status',1)->column('code','id')
	// 返回 item_id -> code
	EnabledItems(ctx context.Context) (map[uint64]string, error)

	// FindUserGroupID 对应 WhitelistUser::where('user_id',$userId)->find()
	FindUserGroupID(ctx context.Context, userID uint64) (groupID uint64, found bool, err error)

	// IsGroupEnabled 对应 WhitelistGroup::where('id',..)->where('status',1)->find()
	IsGroupEnabled(ctx context.Context, groupID uint64) (bool, error)

	// GroupItemEnabled 对应 WhitelistGroupItem::where('group_id',..)->column('enabled','item_id')
	// 返回 item_id -> enabled(0/1)
	GroupItemEnabled(ctx context.Context, groupID uint64) (map[uint64]int, error)

	// PurchasePayWay 取兑换单的支付方式，用于 no_tea_trade / no_tao_trade 的细粒度判断
	PurchasePayWay(ctx context.Context, purchaseID uint64) (payWay int32, found bool, err error)
}

// WhitelistParams 对应原项目传进来的 $request->post()。
//
// 原实现直接把整个 POST body 丢进去，只有 no_tea_trade / no_tao_trade 会读其中几个键。
// 这里收敛成显式字段，避免把「整个请求体」这种模糊概念往 biz 层传。
type WhitelistParams struct {
	// PayWay 创建兑换单时的支付方式
	PayWay *int32
	// PurchaseID 竞价提交 / 兑换人购买藏品时的兑换单 ID
	PurchaseID *uint64
	// OptPwd 与 PurchaseID 同时出现才触发细粒度判断（原代码用 isset 判断）
	OptPwd *string
}

// WhitelistUsecase 白名单用例。
type WhitelistUsecase struct {
	repo  WhitelistRepo
	cache Cache
	log   *log.Helper
}

func NewWhitelistUsecase(repo WhitelistRepo, cache Cache, logger log.Logger) *WhitelistUsecase {
	return &WhitelistUsecase{repo: repo, cache: cache, log: log.NewHelper(logger)}
}

// UserPermissions 对应 WhitelistLogic::getUserPermissions（带 5 分钟缓存）。
//
// 返回 code -> 是否受限。**true 表示该用户被禁止做这类操作**。
//
// ⚠️ 语义容易搞反，所以再说一遍调用链：
//
//	WhitelistMiddleware: if (checkPermissionWithContext(...)) { return fail('白名单用户-暂无该操作权限'); }
//
// 也就是说 checkPermissionWithContext 返回 **true = 拦截**。
func (uc *WhitelistUsecase) UserPermissions(ctx context.Context, userID uint64) (map[string]bool, error) {
	cacheKey := whitelistPermCacheKey(userID)

	if v, err := uc.cache.Get(ctx, cacheKey); err != nil {
		uc.log.WithContext(ctx).Warnf("读取白名单缓存失败: %v", err)
	} else if m, ok := v.(map[string]any); ok {
		out := make(map[string]bool, len(m))
		for k, val := range m {
			out[k] = PhpTruthy(val)
		}
		return out, nil
	}

	perms, err := uc.loadUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := uc.cache.Set(ctx, cacheKey, perms, whitelistCacheTTL); err != nil {
		uc.log.WithContext(ctx).Warnf("写入白名单缓存失败: %v", err)
	}
	return perms, nil
}

// loadUserPermissions 逐行对照 WhitelistLogic::loadUserPermissions。
//
// 关键点：**默认全部放行**，只有在「组存在 + 组启用 + 该组显式配置了某项且 enabled 非 0」
// 时才标记为受限。
func (uc *WhitelistUsecase) loadUserPermissions(ctx context.Context, userID uint64) (map[string]bool, error) {
	items, err := uc.repo.EnabledItems(ctx)
	if err != nil {
		return nil, err
	}

	// PHP: 默认所有限制项都返回 false（不在白名单中的用户不受限制）
	perms := make(map[string]bool, len(items))
	for _, code := range items {
		perms[code] = false
	}

	groupID, found, err := uc.repo.FindUserGroupID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return perms, nil
	}

	ok, err := uc.repo.IsGroupEnabled(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return perms, nil
	}

	groupItems, err := uc.repo.GroupItemEnabled(ctx, groupID)
	if err != nil {
		return nil, err
	}
	for itemID, code := range items {
		if enabled, configured := groupItems[itemID]; configured {
			perms[code] = enabled != 0
		}
		// 组里没配置该项 -> 保持 false（放行）
	}
	return perms, nil
}

// CheckPermission 对应 WhitelistLogic::checkPermission。
//
// ⚠️⚠️ 这里的返回值和它的注释是**矛盾**的，且矛盾是原项目自带的：
//
//	// 如果限制项不存在于系统中，默认放行
//	if (!isset($permissions[$itemCode])) {
//	    return true;                  // ← 注释说放行，但 true 在调用方是"拦截"
//	}
//
// 调用方 WhitelistMiddleware 把 true 当作拦截，所以「限制项不在启用列表里」
// 实际会导致**所有人都被拦**。
//
// 当前不构成线上故障，因为路由用到的 3 个 code（no_collection_trade /
// no_tea_trade / no_tao_trade）在 x_whitelist_item 里都存在且 status=1，
// 走不到这个分支。已在 README 记为潜在缺陷。
//
// ✅ 第 41 轮按"**不改动原项目逻辑**"的要求**改回原字面行为**：
// 未知限制项 -> 返回 true（拦截）。
// 我此前按注释意图实现成"放行"，那是一处有意的行为分歧，现已撤销。
// 单测 `TestWhitelist_UnknownItemIsBlocked` 锁住当前行为。
//
// ⚠️ 后果（知情保留）：若某天有人把某个限制项的 status 停用（或在 x_whitelist_item
// 里删掉），挂该限制项的路由会对**所有人**返回 403 并提示"白名单用户-暂无该操作权限"。
// 这是原实现的行为，不是迁移引入的。
func (uc *WhitelistUsecase) CheckPermission(ctx context.Context, userID uint64, itemCode string) (bool, error) {
	perms, err := uc.UserPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	restricted, known := perms[itemCode]
	if !known {
		// 逐字还原原代码：return true（true 在调用方 = 拦截）
		uc.log.WithContext(ctx).Debugf("白名单限制项未启用，按原代码行为拦截: %s", itemCode)
		return true, nil
	}
	return restricted, nil
}

// CheckPermissionWithContext 对应 WhitelistLogic::checkPermissionWithContext。
//
// 返回 true = 拦截。
//
// 细粒度逻辑（原样保留，逻辑确实绕）：
//   - no_tea_trade：带 pay_way 且 pay_way != 1 → **放行**（交给 no_tao_trade 去拦）
//     带 purchase_id+opt_pwd 且该单 pay_way != 1 → 放行
//   - no_tao_trade：对称，pay_way != 2 → 放行
//
// 换句话说：只有「真正要用被禁的那种支付方式」时才拦。
func (uc *WhitelistUsecase) CheckPermissionWithContext(
	ctx context.Context, userID uint64, itemCode string, p WhitelistParams,
) (bool, error) {
	restricted, err := uc.CheckPermission(ctx, userID, itemCode)
	if err != nil {
		return false, err
	}
	if !restricted {
		return false, nil
	}

	switch itemCode {
	case WhitelistItemNoTeaTrade:
		if p.PayWay != nil && !PhpLooseEqualsInt(*p.PayWay, 1) {
			return false, nil
		}
		if p.PurchaseID != nil && p.OptPwd != nil {
			payWay, found, err := uc.repo.PurchasePayWay(ctx, *p.PurchaseID)
			if err != nil {
				return false, err
			}
			if found && !PhpLooseEqualsInt(payWay, 1) {
				return false, nil
			}
		}
	case WhitelistItemNoTaoTrade:
		if p.PayWay != nil && !PhpLooseEqualsInt(*p.PayWay, 2) {
			return false, nil
		}
		if p.PurchaseID != nil && p.OptPwd != nil {
			payWay, found, err := uc.repo.PurchasePayWay(ctx, *p.PurchaseID)
			if err != nil {
				return false, err
			}
			if found && !PhpLooseEqualsInt(payWay, 2) {
				return false, nil
			}
		}
	}
	return true, nil
}

func whitelistPermCacheKey(userID uint64) string {
	// 与 PHP 的 'whitelist:user_perm:' . $userId 逐字一致（前缀由 Cache 实现加）
	return "whitelist:user_perm:" + strconv.FormatUint(userID, 10)
}
