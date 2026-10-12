package biz

import (
	"context"
	"encoding/json"
)

// AppUserGoodsUsecase 对应 app/api/lists/open/AppUserGoodsLists.php。
type AppUserGoodsUsecase struct {
	repo AppUserGoodsRepo
}

func NewAppUserGoodsUsecase(repo AppUserGoodsRepo) *AppUserGoodsUsecase {
	return &AppUserGoodsUsecase{repo: repo}
}

// AppUserGoodsRepo 一级平台用户商品。
type AppUserGoodsRepo interface {
	// ListGoods 对应 lists() —— **四张表**：
	//   MarketSales
	//     INNER JOIN x_app_archive AppArchive ON AppArchive.id = MarketSales.archive_id   (hasWhere('archive'))
	//     INNER JOIN x_app        app         ON MarketSales.app_id = app.id
	//     INNER JOIN x_app_auth   app_auth    ON MarketSales.app_auth_id = app_auth.id
	ListGoods(ctx context.Context, q AppUserGoodsQuery, offset, limit int) ([]map[string]any, error)
	// CountGoods 对应 count() —— 同一个 hasWhere('archive')，**但只有那一个 JOIN**
	CountGoods(ctx context.Context, q AppUserGoodsQuery) (int64, error)
	// FindAppAuthID 对应 queryWhere() 里的
	//   AppAuth::getAppAuthInfo($userId, $app_id, $app_uid)
	FindAppAuthID(ctx context.Context, userID uint32, appID int64, appUID string) (int64, bool, error)
}

// AppUserGoodsQuery 对应 queryWhere() 的条件。
type AppUserGoodsQuery struct {
	UserID uint32
	// Scene 对应 `$this->params['scene']`：
	//   1 -> state='OFFLINE'
	//   2 -> state='ON_SALE' AND sale_lock <> 'LOCK'
	//   3 -> state='ON_SALE' AND sale_lock =  'LOCK'
	//   其它/未传 -> state='OFFLINE'
	Scene int32
	// AppID 对应 `$this->params['app_id']`（!empty 才生效）
	AppID int64
	// AppAuthID 由 AppAuth 查出来后填入；0 表示不筛
	AppAuthID int64
	// AppUID 对应 `$this->params['app_uid']`（!empty 才触发 AppAuth 查询）
	AppUID string
}

// 逐字对应 scene switch 里的字面量
const (
	SalesStateOffline = "OFFLINE"
	SalesStateOnSale  = "ON_SALE"
	SalesLockLock     = "LOCK"
)

// AppUserGoodsSelect 逐字对应 lists() 的 field（12 项）。
//
// ⚠️ **不含 `state` / `sale_lock`** —— 虽然 where 里筛了它们，但输出里没有。
const AppUserGoodsSelect = "MarketSales.id AS id, MarketSales.sn AS sn, " +
	"MarketSales.price AS price, MarketSales.app_auth_id AS app_auth_id, " +
	"MarketSales.collection_id AS collection_id, " +
	"AppArchive.id AS archive_id, AppArchive.name AS name, " +
	"AppArchive.issuer AS issuer, AppArchive.images AS images, " +
	"AppArchive.app_id AS app_id, " +
	"app.name AS platform_name, app_auth.app_uid AS app_uid"

// List 对应 `dataLists(new AppUserGoodsLists())`。
//
// 原文 queryWhere()：
//
//	$where[] = ['MarketSales.user_id','=',$this->userId];
//	if (!empty($this->params['scene'])) {
//	    switch ($this->params['scene']) {
//	        case 1: $where[] = ['MarketSales.state','=','OFFLINE']; break;                 // 待上架
//	        case 2: $where[] = ['MarketSales.state','=','ON_SALE'];
//	                $where[] = ['MarketSales.sale_lock','<>','LOCK']; break;               // 销售中
//	        case 3: $where[] = ['MarketSales.state','=','ON_SALE'];
//	                $where[] = ['MarketSales.sale_lock','=','LOCK']; break;                // 兑换中
//	        default: $where[] = ['MarketSales.state','=','OFFLINE']; break;
//	    }
//	} else {
//	    $where[] = ['MarketSales.state','=','OFFLINE'];
//	}
//	if (!empty($this->params['app_id']))  { $where[] = ['MarketSales.app_id','=',$this->params['app_id']]; }
//	if (!empty($this->params['app_uid'])) {
//	    $appAuthInfo = AppAuth::getAppAuthInfo($this->userId, $this->params['app_id'], $this->params['app_uid']);
//	    if ($appAuthInfo) { $where[] = ['app_auth_id','=', $appAuthInfo['id']]; }
//	}
//
// ⚠️ **scene 的 default 分支 = case 1**（都筛 OFFLINE），所以未传 scene 与 scene=1 等价。
//
// ⚠️⚠️ `app_uid` 那段的**静默失败**：查不到授权记录时**不追加任何条件** ——
//
//	于是会返回该用户的**全部**商品（而不是空）。照搬，不要"修正"成返回空。
//
// ⚠️ `!empty()` 语义：scene/app_id/app_uid 为 0 或 ” 都视为"未传"。
func (uc *AppUserGoodsUsecase) List(
	ctx context.Context, q AppUserGoodsQuery, p PageParams,
) (*ListData, error) {
	// app_uid 触发的授权查询（在 where 构造阶段，与分页无关）
	if q.AppUID != "" {
		id, found, err := uc.repo.FindAppAuthID(ctx, q.UserID, q.AppID, q.AppUID)
		if err != nil {
			return nil, err
		}
		if found {
			q.AppAuthID = id
		}
		// 查不到 -> 不加条件（原文行为）
	}

	count, err := uc.repo.CountGoods(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListGoods(ctx, q, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}

	// 逐行：if (is_string($item['images'])) { $item['images'] = json_decode($item['images'], true); }
	for _, m := range rows {
		if s, ok := m["images"].(string); ok {
			var v any
			if json.Unmarshal([]byte(s), &v) == nil {
				m["images"] = v
			}
		}
	}

	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}
