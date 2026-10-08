package biz

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 白名单的语义很容易搞反（函数返回 true 表示"拦截"），
// 而且原实现里 `checkPermission` 的注释与代码互相矛盾。
// 所以这里把边界全部钉死，尤其是那处有意的行为分歧。

/* ---------------------------------------------------------------- 假实现 */

type fakeWhitelistRepo struct {
	items      map[uint64]string
	groupID    uint64
	hasUser    bool
	groupOK    bool
	groupItems map[uint64]int
	// purchaseID -> pay_way
	purchases map[uint64]int32
}

func (f *fakeWhitelistRepo) EnabledItems(context.Context) (map[uint64]string, error) {
	return f.items, nil
}

func (f *fakeWhitelistRepo) FindUserGroupID(context.Context, uint64) (uint64, bool, error) {
	return f.groupID, f.hasUser, nil
}

func (f *fakeWhitelistRepo) IsGroupEnabled(context.Context, uint64) (bool, error) {
	return f.groupOK, nil
}

func (f *fakeWhitelistRepo) GroupItemEnabled(context.Context, uint64) (map[uint64]int, error) {
	return f.groupItems, nil
}

func (f *fakeWhitelistRepo) PurchasePayWay(_ context.Context, id uint64) (int32, bool, error) {
	if pw, ok := f.purchases[id]; ok {
		return pw, true, nil
	}
	return 0, false, nil
}

func newWhitelistUsecase(repo *fakeWhitelistRepo) *WhitelistUsecase {
	return NewWhitelistUsecase(repo, newFakeTokenCacheForWL(), log.NewStdLogger(io.Discard))
}

// 复用 token 测试里的假缓存：白名单缓存存的是 map[string]any
type fakeWLCache struct {
	m map[string]any
}

func newFakeTokenCacheForWL() *fakeWLCache { return &fakeWLCache{m: map[string]any{}} }

func (f *fakeWLCache) Get(_ context.Context, k string) (any, error) { return f.m[k], nil }

func (f *fakeWLCache) Set(_ context.Context, k string, v any, _ time.Duration) error {
	f.m[k] = v
	return nil
}

// 五个限制项都在，模拟 x_whitelist_item 的真实数据
func allItems() map[uint64]string {
	return map[uint64]string{
		1: WhitelistItemNoCollectionTrade,
		2: WhitelistItemNoWithdraw,
		3: WhitelistItemNoSync,
		4: WhitelistItemNoTeaTrade,
		5: WhitelistItemNoTaoTrade,
	}
}

/* ---------------------------------------------------------------- 用例 */

// 不在任何白名单组里 -> 全部放行
func TestWhitelist_UserWithoutGroupIsAllowed(t *testing.T) {
	repo := &fakeWhitelistRepo{items: allItems(), hasUser: false}
	uc := newWhitelistUsecase(repo)

	blocked, err := uc.CheckPermission(context.Background(), 42, WhitelistItemNoCollectionTrade)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if blocked {
		t.Fatal("不在白名单组的用户不应被拦截")
	}
}

// 在组里，但组被停用 -> 放行
func TestWhitelist_DisabledGroupIsAllowed(t *testing.T) {
	repo := &fakeWhitelistRepo{items: allItems(), hasUser: true, groupID: 68, groupOK: false}
	uc := newWhitelistUsecase(repo)

	blocked, _ := uc.CheckPermission(context.Background(), 42, WhitelistItemNoCollectionTrade)
	if blocked {
		t.Fatal("组被停用时不应拦截")
	}
}

// 组启用 + 该项 enabled=1 -> 拦截
func TestWhitelist_EnabledItemBlocks(t *testing.T) {
	repo := &fakeWhitelistRepo{
		items: allItems(), hasUser: true, groupID: 68, groupOK: true,
		groupItems: map[uint64]int{1: 1},
	}
	uc := newWhitelistUsecase(repo)

	blocked, err := uc.CheckPermission(context.Background(), 42, WhitelistItemNoCollectionTrade)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if !blocked {
		t.Fatal("enabled=1 的限制项必须拦截")
	}
	// 组里没配的其他项仍然放行
	blocked2, _ := uc.CheckPermission(context.Background(), 42, WhitelistItemNoWithdraw)
	if blocked2 {
		t.Fatal("组里没配置的限制项应当放行")
	}
}

// 组启用但 enabled=0 -> 放行（当前线上就是这个状态：9 条配置全是 0）
func TestWhitelist_DisabledItemAllowed(t *testing.T) {
	repo := &fakeWhitelistRepo{
		items: allItems(), hasUser: true, groupID: 69, groupOK: true,
		groupItems: map[uint64]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	}
	uc := newWhitelistUsecase(repo)

	for _, code := range []string{
		WhitelistItemNoCollectionTrade, WhitelistItemNoWithdraw, WhitelistItemNoSync,
		WhitelistItemNoTeaTrade, WhitelistItemNoTaoTrade,
	} {
		blocked, _ := uc.CheckPermission(context.Background(), 42, code)
		if blocked {
			t.Fatalf("enabled=0 的限制项 %s 不应拦截", code)
		}
	}
}

// 未知限制项 —— **逐字还原原代码行为：返回 true（拦截）**。
//
// 原代码这里是 `return true`，而它上面的注释写的是"默认放行" —— 注释与代码相反。
// 第 41 轮按"不改动原项目逻辑"的要求撤销了我此前"按注释意图放行"的实现。
//
// 这条测试锁住的是**原项目的字面行为**（含它的缺陷），不是我认为对的行为。
// 后果：若某限制项被停用或从 x_whitelist_item 删除，挂它的路由会对所有人 403。
func TestWhitelist_UnknownItemIsBlocked(t *testing.T) {
	repo := &fakeWhitelistRepo{items: allItems(), hasUser: false}
	uc := newWhitelistUsecase(repo)

	blocked, err := uc.CheckPermission(context.Background(), 42, "no_such_item")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if !blocked {
		t.Fatal("未知限制项应当**拦截**（原代码 return true；注释虽写『放行』，但行为以代码为准）")
	}
}

/* ------------------------------------------------- 细粒度（no_tea_trade / no_tao_trade） */

func restrictedTo(code string, itemID uint64) *WhitelistUsecase {
	return newWhitelistUsecase(&fakeWhitelistRepo{
		items: allItems(), hasUser: true, groupID: 68, groupOK: true,
		groupItems: map[uint64]int{itemID: 1},
	})
}

func i32(v int32) *int32    { return &v }
func u64(v uint64) *uint64  { return &v }
func strp(v string) *string { return &v }

// 被禁茶交易的用户，用 pay_way=1（茶）要拦；用 pay_way=2（陶）放行，
// 因为那是 no_tao_trade 该管的事。
func TestWhitelist_NoTeaTrade_PayWay(t *testing.T) {
	uc := restrictedTo(WhitelistItemNoTeaTrade, 4)
	ctx := context.Background()

	blocked, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTeaTrade,
		WhitelistParams{PayWay: i32(1)})
	if !blocked {
		t.Fatal("pay_way=1 时应当拦截")
	}

	blocked2, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTeaTrade,
		WhitelistParams{PayWay: i32(2)})
	if blocked2 {
		t.Fatal("pay_way=2 时应当放行（交给 no_tao_trade 处理）")
	}
}

func TestWhitelist_NoTaoTrade_PayWay(t *testing.T) {
	uc := restrictedTo(WhitelistItemNoTaoTrade, 5)
	ctx := context.Background()

	blocked, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTaoTrade,
		WhitelistParams{PayWay: i32(2)})
	if !blocked {
		t.Fatal("pay_way=2 时应当拦截")
	}

	blocked2, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTaoTrade,
		WhitelistParams{PayWay: i32(1)})
	if blocked2 {
		t.Fatal("pay_way=1 时应当放行")
	}
}

// 通过 purchase_id + opt_pwd 触发细粒度：要查兑换单本身的 pay_way
func TestWhitelist_NoTeaTrade_ByPurchase(t *testing.T) {
	repo := &fakeWhitelistRepo{
		items: allItems(), hasUser: true, groupID: 68, groupOK: true,
		groupItems: map[uint64]int{4: 1},
		purchases:  map[uint64]int32{1001: 1, 1002: 2},
	}
	uc := newWhitelistUsecase(repo)
	ctx := context.Background()

	// 兑换单 pay_way=1 -> 拦截
	blocked, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTeaTrade,
		WhitelistParams{PurchaseID: u64(1001), OptPwd: strp("x")})
	if !blocked {
		t.Fatal("兑换单 pay_way=1 时应当拦截")
	}

	// 兑换单 pay_way=2 -> 放行
	blocked2, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTeaTrade,
		WhitelistParams{PurchaseID: u64(1002), OptPwd: strp("x")})
	if blocked2 {
		t.Fatal("兑换单 pay_way=2 时应当放行")
	}

	// 只给 purchase_id 不给 opt_pwd 时不触发细粒度（原代码用 isset 同时判断两者）
	blocked3, _ := uc.CheckPermissionWithContext(ctx, 42, WhitelistItemNoTeaTrade,
		WhitelistParams{PurchaseID: u64(1002)})
	if !blocked3 {
		t.Fatal("缺少 opt_pwd 时不应走到细粒度分支，应当拦截")
	}
}

// 没有限制项的用户，细粒度逻辑完全不参与
func TestWhitelist_NotRestrictedNeverBlocks(t *testing.T) {
	repo := &fakeWhitelistRepo{items: allItems(), hasUser: false}
	uc := newWhitelistUsecase(repo)

	blocked, _ := uc.CheckPermissionWithContext(context.Background(), 42,
		WhitelistItemNoTeaTrade, WhitelistParams{PayWay: i32(1)})
	if blocked {
		t.Fatal("无限制的用户不应被拦截")
	}
}
