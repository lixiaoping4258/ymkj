package biz

import (
	"context"
	"encoding/json"
)

// SaleRecordUsecase 对应 app/api/lists/market/sales/SaleRecordLists.php。
type SaleRecordUsecase struct {
	repo SaleRecordRepo
}

func NewSaleRecordUsecase(repo SaleRecordRepo) *SaleRecordUsecase {
	return &SaleRecordUsecase{repo: repo}
}

// SaleRecordRepo 秒转兑出记录列表。
type SaleRecordRepo interface {
	// ListRecords 对应 lists() —— **4 张表**：
	//   sales_orders_detail SalesOrdersDetail
	//     INNER JOIN x_app_archive   AppArchive   ON SalesOrdersDetail.archive_id = AppArchive.id
	//     INNER JOIN x_market_sales  market_sales ON market_sales.id = SalesOrdersDetail.sale_id
	//     INNER JOIN x_sales_orders  SalesOrders  ON SalesOrders.id = SalesOrdersDetail.order_id
	ListRecords(ctx context.Context, q SaleRecordQuery, orderBy string, offset, limit int) ([]map[string]any, error)
	// CountRecords 对应 count() —— ⚠️⚠️ **只有 2 张表**（少了 market_sales 和 SalesOrders）
	CountRecords(ctx context.Context, q SaleRecordQuery) (int64, error)
}

// SaleRecordQuery 对应 queryWhere()。
//
//	$where = [['SalesOrdersDetail.user_id','=',$this->userId]];
//	if (!empty($keyword)) { $where[] = ['AppArchive.name','like','%'.$keyword.'%']; }
//	if (!empty($state))   { $where[] = ['SalesOrdersDetail.state','=',$state]; }
type SaleRecordQuery struct {
	UserID  uint32
	Keyword string
	State   string
}

// SaleRecordSort 对应 lists() 的 sort switch。
//
// ⚠️⚠️ **只有 time_asc / time_desc 两个分支**（与同目录的 SaleOrderLists /
//
//	SaleGoodsLists **不同** —— 那两个还有 price_asc / price_desc）。
//	这里根本没有 price 分支，所以**不存在 total_price 那个错误**。
//	默认（含未知值）= `SalesOrdersDetail.start_time desc`。
func SaleRecordSort(raw string) string {
	switch raw {
	case "time_asc":
		return "SalesOrdersDetail.start_time asc"
	case "time_desc":
		return "SalesOrdersDetail.start_time desc"
	default:
		return "SalesOrdersDetail.start_time desc"
	}
}

// SaleRecordSelect 逐字对应 lists() 的 field（6 项）。
//
// ⚠️ `SalesOrders.order_no` 来自第 4 张表。
const SaleRecordSelect = "SalesOrdersDetail.id AS id, SalesOrdersDetail.price AS price, " +
	"SalesOrdersDetail.state AS state, SalesOrders.order_no AS order_no, " +
	"AppArchive.name AS archive_name, AppArchive.images AS archive_images"

// List 对应 `dataLists(new SaleRecordLists())`。
//
// ⚠️⚠️⚠️ **`lists()` 与 `count()` 用的 JOIN 数量不一样**（清单第 3 条，**第 4 个实例**）：
//
//	lists():  sales_orders_detail + app_archive + market_sales + sales_orders   (4 张)
//	count():  sales_orders_detail + app_archive                                (2 张)
//
// **后果**：`count()` 会把 `lists()` 里被后两个 INNER JOIN 过滤掉的行也算进去，
// 所以 **count ≥ lists 实际行数**。分页会显示"共 N 条"但只列出更少的行。
//
// 已实测（user_id=13 时）：
//
//	2 表（count 的口径）: 2 行
//	4 表（lists 的口径）: 1 行
//
// 全库 `x_sales_orders_detail` 有 2 行，其中 1 行的 `sale_id` 在
// `x_market_sales` 里**找不到**（孤儿），所以被 lists 的 INNER JOIN 丢掉。
//
// **照搬**，不把 count 也加上后两个 JOIN。
func (uc *SaleRecordUsecase) List(ctx context.Context, q SaleRecordQuery, sortRaw string, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountRecords(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListRecords(ctx, q, SaleRecordSort(sortRaw), p.Offset, p.Limit)
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
