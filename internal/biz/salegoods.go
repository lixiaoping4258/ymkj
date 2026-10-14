package biz

import (
	"context"
	"encoding/json"
)

// SaleGoodsUsecase 对应 app/api/lists/market/sales/SaleGoodsLists.php。
//
// ⚠️ 它与 SaleOrderLists **看着很像但不是同一张表**：
//
//	SaleOrderLists:  SalesOrders  + sales_orders_detail + app_archive     (三表)
//	SaleGoodsLists:  MarketSales  + app_archive                          (两表)
//
// 而且两者的 `sort` 白名单**映射到不同的表别名**（SaleGoodsLists 用 `MarketSales.`）。
// 不要复用同一个映射函数。
type SaleGoodsUsecase struct {
	repo SaleGoodsRepo
}

func NewSaleGoodsUsecase(repo SaleGoodsRepo) *SaleGoodsUsecase {
	return &SaleGoodsUsecase{repo: repo}
}

// SaleGoodsRepo 秒转「我的兑出」商品列表。
type SaleGoodsRepo interface {
	// ListGoods 对应 lists() —— 两张表：
	//   MarketSales INNER JOIN x_app_archive AppArchive ON MarketSales.archive_id = AppArchive.id
	ListGoods(ctx context.Context, q SaleGoodsQuery, orderBy string, offset, limit int) ([]map[string]any, error)
	// CountGoods 对应 count() —— **同样的 JOIN**
	CountGoods(ctx context.Context, q SaleGoodsQuery) (int64, error)
}

// SaleGoodsQuery 对应 queryWhere()。
//
//	$where = [['MarketSales.user_id','=',$this->userId]];
//	if (!empty($this->params['keyword'])) {
//	    $where[] = ['AppArchive.name', 'like', '%'.$this->params['keyword'].'%'];
//	}
//
// ⚠️ LIKE 打的是 **`AppArchive.name`**（不是 MarketSales 的列）。
type SaleGoodsQuery struct {
	UserID  uint32
	Keyword string
}

// SaleGoodsSort 对应 lists() 里的 sort switch（默认 `time_desc`）。
//
// 🔴🔴 **`MarketSales.total_price` 这一列不存在**
//
//	（`x_market_sales` 的实际列是 id,app_id,user_id,app_auth_id,archive_id,sn,
//	  collection_id,**price**,app_fee,create_time,state,publish_time,content,
//	  hash,sale_lock,sale_state —— 没有 total_price）。
//
//	所以 `sort=price_asc` / `price_desc` 会报
//	  `Error 1054: Unknown column 'MarketSales.total_price' in 'order clause'`
//	—— **与 SaleOrderLists 是同一个错**（那边是 SalesOrders.total_price）。
//	照搬，不改成 `price`。
//
// ⚠️ 只有这 4 个白名单值；其它值走 default（`MarketSales.create_time desc`）。
func SaleGoodsSort(raw string) string {
	switch raw {
	case "price_asc":
		return "MarketSales.total_price asc"
	case "price_desc":
		return "MarketSales.total_price desc"
	case "time_asc":
		return "MarketSales.create_time asc"
	case "time_desc":
		return "MarketSales.create_time desc"
	default:
		return "MarketSales.create_time desc"
	}
}

// SaleGoodsSelect 逐字对应 lists() 的 field（5 项）。
const SaleGoodsSelect = "MarketSales.id AS id, MarketSales.price AS price, " +
	"MarketSales.state AS state, AppArchive.name AS archive_name, " +
	"AppArchive.images AS archive_images"

// List 对应 `dataLists(new SaleGoodsLists())`。
//
// ⚠️ 两个 JOIN（这里只有一个）都是 **INNER JOIN** —— archive_id 在
//
//	`x_app_archive` 里不存在的商品**不会出现**；`count()` 用同样的 JOIN，两者一致。
//
// ⚠️ `archive_images` 只在 **is_string** 时 json_decode（没有 else）——
//
//	与 SaleOrderLists 相同风格。
//
// ⚠️ MarketSales **无软删除**（模型未 use SoftDelete，表也没有 delete_time）。
func (uc *SaleGoodsUsecase) List(ctx context.Context, q SaleGoodsQuery, sortRaw string, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountGoods(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListGoods(ctx, q, SaleGoodsSort(sortRaw), p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		if s, ok := m["archive_images"].(string); ok {
			var v any
			if json.Unmarshal([]byte(s), &v) == nil {
				m["archive_images"] = v
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
