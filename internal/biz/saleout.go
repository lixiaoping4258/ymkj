package biz

import (
	"context"
	"encoding/json"
)

// SaleOutUsecase 对应 app/api/lists/market/sales/SaleOutCollectionLists.php。
//
// ⚠️⚠️ 它与 SaleOnCollectionLists **看着对称，实则有三处不同**（已用 diff 逐行核对）：
//
//	             sale/on                      sale/out
//	查找        MarketListSalesLogic::        SaleLogic::
//	            findSales($id)               getArchive($id)
//	查的是      x_market_list_sales.id        x_app_archive.id（且 state=1）
//	错误文案    '记录不存在'                   '存档记录不存在'
//	状态        'ON_SALE'                    'SELL_OUT'
//	额外字段    —                            $item['sale_time_text'] = ''（硬编码空串）
//
// **不要因为"长得对称"就复制代码。**
//
// ⚠️ `archive_id` 取的是 `$archive['id']`（就是入参本身），
//
//	而不是像 sale/on 那样从列表记录里取 archive_id。
type SaleOutUsecase struct {
	repo SaleOnRepo
}

func NewSaleOutUsecase(repo SaleOnRepo) *SaleOutUsecase {
	return &SaleOutUsecase{repo: repo}
}

// ErrSaleOutArchiveNotFound 对应构造里的 `throw new \Exception('存档记录不存在')`。
//
// ⚠️ 注意与 `SaleLogic::getArchive` 内部的 `setError('未找到档案信息')` **不是同一条**：
//
//	构造函数把 `getArchive` 的 false 吞掉，统一抛 '存档记录不存在'。
var ErrSaleOutArchiveNotFound = errSaleOutNotFound{}

type errSaleOutNotFound struct{}

func (errSaleOutNotFound) Error() string { return "存档记录不存在" }

// ListOut 对应 `dataLists(new SaleOutCollectionLists())`。
//
// 构造函数：
//
//	$id = $this->params['id'] ?? '';
//	$archive = SaleLogic::getArchive($id);          // ⚠️ 不是 findSales
//	if ($archive === false) { throw new \Exception('存档记录不存在'); }
//	$this->params['archive_id'] = $archive['id'];
//
// queryWhere():
//
//	[['MarketSales.archive_id','=',$archive_id], ['MarketSales.state','=','SELL_OUT']]
//
// lists()：
//
//	MarketSales::hasWhere('archive', $where, <与 sale/on 相同的 7 字段>)
//	    ->join('app','MarketSales.app_id=app.id')
//	    ->order('id desc')->limit(...)->select();
//	逐行：
//	    $item['sale_time_text'] = '';               // ⚠️ 硬编码空串
//	    if (is_string($item['images'])) { json_decode(...); }
//
// count()：同样的 `hasWhere('archive')` 单 JOIN（lists 是两个）。
//
// ⚠️ `archiveID` 由调用方通过 `GetSaleArchive(id)` 得到（`state=1` 的档案）。
func (uc *SaleOutUsecase) List(ctx context.Context, archiveID int64, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountByState(ctx, archiveID, SalesStateSellOut)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListByState(ctx, archiveID, SalesStateSellOut, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		// ⚠️ 硬编码空串 —— 不是查出来的（原字段可能是后来废弃的）
		m["sale_time_text"] = ""
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
