package biz

import (
	"context"
	"encoding/json"
)

// SaleOnUsecase 对应 app/api/lists/market/sales/SaleOnCollectionLists.php。
type SaleOnUsecase struct {
	repo SaleOnRepo
}

func NewSaleOnUsecase(repo SaleOnRepo) *SaleOnUsecase {
	return &SaleOnUsecase{repo: repo}
}

// ErrSaleOnRecordNotFound 对应构造里的 `throw new Exception('记录不存在')`。
//
// ⚠️ 原文是 **throw**（不是 `self::setError` + return false）——
//
//	ThinkPHP 会把异常转成错误响应，与 `fail()` 的形态未必一致。
//	这里按 `fail('记录不存在')` 实现（与 purchase_state.go 里同类情形的处理一致），
//	**接前若发现前端依赖异常的响应形态，需要单独核对**。
var ErrSaleOnRecordNotFound = errSaleOnNotFound{}

type errSaleOnNotFound struct{}

func (errSaleOnNotFound) Error() string { return "记录不存在" }

// SaleOnRepo 秒转在售藏品。
type SaleOnRepo interface {
	// ListOnSale 对应 lists() —— **两张表**：
	//   MarketSales INNER JOIN x_app_archive AppArchive (hasWhere('archive'))
	//               INNER JOIN x_app         app          (join('app'))
	ListOnSale(ctx context.Context, archiveID int64, offset, limit int) ([]map[string]any, error)
	// CountOnSale 对应 count() —— ⚠️ **只有 hasWhere('archive') 一个 JOIN**
	CountOnSale(ctx context.Context, archiveID int64) (int64, error)
}

// SaleOnSelect 逐字对应 lists() 的 field（7 项）。
//
// ⚠️ 不含 `state`（虽然 where 里筛了它）。
const SaleOnSelect = "MarketSales.id AS id, MarketSales.sn AS sn, MarketSales.price AS price, " +
	"AppArchive.name AS name, AppArchive.issuer AS issuer, AppArchive.images AS images, " +
	"app.name AS platform_name"

// List 对应 `dataLists(new SaleOnCollectionLists())`。
//
// 原文构造函数：
//
//	$id = $this->params['id'] ?? '';
//	$collection = MarketListSalesLogic::findSales($id);
//	if (empty($collection)) { throw new Exception('记录不存在'); }
//	$this->params['archive_id'] = $collection['archive_id'];
//
// queryWhere():
//
//	[['MarketSales.archive_id','=',$archive_id], ['MarketSales.state','=','ON_SALE']]
//
// lists():
//
//	MarketSales::hasWhere('archive', $where, <7 字段>)
//	    ->join('app','MarketSales.app_id=app.id')
//	    ->order('id desc')->limit(...)->select()->toArray();
//	// 逐行：if (is_string($list['images'])) { json_decode(...); }
//
// count():
//
//	MarketSales::hasWhere('archive', $this->queryWhere())->count();
//
// 🔴 **lists 有两个 JOIN、count 只有一个**（清单第 3 条），照搬。
//
// ⚠️ 原文里"查询存档记录"那段（`SaleLogic::getArchive`）**被注释掉了** ——
//
//	所以只校验了 `findSales`，没有校验档案是否存在。
//
// ⚠️ `archiveID` 由调用方通过 `findSales(id)` 得到后传入。
func (uc *SaleOnUsecase) List(ctx context.Context, archiveID int64, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountOnSale(ctx, archiveID)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListOnSale(ctx, archiveID, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	// if (is_string($list['images'])) { json_decode(...) }
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
