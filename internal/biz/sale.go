package biz

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/market/SaleController::index / categories
//   app/api/lists/market/sales/SaleFaceLists

// SaleCategories 对应 SaleController::categories —— 硬编码，不查库。
//
// 原实现：
//
//	$data = [['title' => '全部', 'key' => 'all']];
//	return $this->data($data);
//
// 保持硬编码是刻意的：它只是给前端一个"全部分类"的占位项。
func SaleCategories() []SaleCategory {
	return []SaleCategory{{Title: "全部", Key: "all"}}
}

type SaleCategory struct {
	Title string
	Key   string
}

// SaleFaceQuery 是秒转专区列表的查询条件。
type SaleFaceQuery struct {
	Page       PageParams
	Keyword    string
	PriceStart float64
	PriceEnd   float64
	TimeStart  string
	TimeEnd    string
	Sort       string
}

// SaleFaceRow 是列表的一行。
type SaleFaceRow struct {
	ID           int64
	Name         string
	Images       *string
	Issuer       string
	IssuerTime   *time.Time
	SaleAmount   int64
	LowPrice     *string // decimal(10,2)
	MaxPrice     *string
	PlatformName string
}

// ToMap 输出的键与类型必须与 PHP 逐字一致。
//
// 与 PurchaseFaceRow.ToMap 的区别：**这里没有 "0 转 --" 那套后处理**，
// 只有 images 的 json 解码（原实现的 foreach 里只有这一件事）。
func (r SaleFaceRow) ToMap() map[string]any {
	return map[string]any{
		"id":            r.ID,
		"name":          r.Name,
		"images":        decodeImages(r.Images),
		"issuer":        r.Issuer,
		"issuer_time":   formatDBTime(r.IssuerTime),
		"sale_amount":   r.SaleAmount,
		"low_price":     derefStr(r.LowPrice),
		"max_price":     derefStr(r.MaxPrice),
		"platform_name": r.PlatformName,
	}
}

// SaleFaceRepo 秒转专区列表的数据访问。
type SaleFaceRepo interface {
	List(ctx context.Context, q SaleFaceQuery, offset, limit int) ([]SaleFaceRow, error)
	Count(ctx context.Context, q SaleFaceQuery) (int64, error)
}

// SaleFaceUsecase 秒转专区列表用例。
type SaleFaceUsecase struct {
	repo SaleFaceRepo
	log  *log.Helper
}

func NewSaleFaceUsecase(repo SaleFaceRepo, logger log.Logger) *SaleFaceUsecase {
	return &SaleFaceUsecase{repo: repo, log: log.NewHelper(logger)}
}

// Index 对应 SaleController::index。
//
// 原实现只有一句：
//
//	public function index(): Json { return $this->dataLists(new SaleFaceLists()); }
//
// 比兑换专区简单得多：**没有交易时段判断、没有缓存、没有生产测试用户分支**。
// 所以这里也没有。
func (uc *SaleFaceUsecase) Index(ctx context.Context, q SaleFaceQuery) (json.RawMessage, error) {
	rows, err := uc.repo.List(ctx, q, q.Page.Offset, q.Page.Limit)
	if err != nil {
		return nil, err
	}
	count, err := uc.repo.Count(ctx, q)
	if err != nil {
		return nil, err
	}

	lists := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		lists = append(lists, r.ToMap())
	}
	return ListData{
		Lists:    lists,
		Count:    count,
		PageNo:   q.Page.PageNo,
		PageSize: q.Page.PageSize,
	}.ToJSON()
}
