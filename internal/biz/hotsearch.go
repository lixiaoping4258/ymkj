package biz

import "context"

// HotSearchUsecase 对应 app/api/logic/SearchLogic.php::hotLists()。
type HotSearchUsecase struct {
	cfg  *ConfigUsecase
	repo HotSearchRepo
}

func NewHotSearchUsecase(cfg *ConfigUsecase, repo HotSearchRepo) *HotSearchUsecase {
	return &HotSearchUsecase{cfg: cfg, repo: repo}
}

// HotSearchRepo 热搜表。
type HotSearchRepo interface {
	// ListHotSearch 对应：
	//   HotSearch::field(['name','sort'])
	//       ->order(['sort'=>'desc','id'=>'desc'])->select()->toArray()
	ListHotSearch(ctx context.Context) ([]HotSearchItem, error)
}

// HotSearchItem 对应 field(['name','sort'])。
type HotSearchItem struct {
	Name string
	Sort int32
}

// HotSearchResult 对应 `['status'=>..., 'data'=>...]`。
type HotSearchResult struct {
	// Status 是"功能状态 0-关闭 1-开启"，来自 x_config 的 hot_search.status，
	// **默认值是 0**（配置缺失时关闭）。
	Status int32
	Data   []HotSearchItem
}

// GetHotLists 逐字对应 SearchLogic::hotLists()。
//
// 原文：
//
//	$data = HotSearch::field(['name','sort'])
//	    ->order(['sort'=>'desc','id'=>'desc'])->select()->toArray();
//	return [
//	    'status' => ConfigService::get('hot_search', 'status', 0),
//	    'data'   => $data,
//	];
//
// ⚠️ `ConfigService::get('hot_search','status',0)` 的**默认值是 0（关闭）** ——
//
//	本项目 x_config 里**没有 hot_search 段**，所以 status 恒为 0。
//	也就是说这个接口当前返回的是"热搜功能已关闭 + 空列表"。
//
// ⚠️ 排序是**两个字段**：先 sort desc，sort 相同再 id desc。不能只留一个。
func (uc *HotSearchUsecase) GetHotLists(ctx context.Context) (*HotSearchResult, error) {
	data, err := uc.repo.ListHotSearch(ctx)
	if err != nil {
		return nil, err
	}
	status, err := uc.cfg.GetDefault(ctx, "hot_search", "status", 0)
	if err != nil {
		return nil, err
	}
	return &HotSearchResult{
		Status: int32(toIntAny(status, 0)),
		Data:   data,
	}, nil
}
