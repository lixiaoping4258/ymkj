package biz

import "context"

// OpenAppUsecase 对应 app/api/lists/open/AppLists.php。
type OpenAppUsecase struct {
	repo OpenAppRepo
}

func NewOpenAppUsecase(repo OpenAppRepo) *OpenAppUsecase {
	return &OpenAppUsecase{repo: repo}
}

// OpenAppRepo 三方平台（x_app）。
type OpenAppRepo interface {
	// ListApps 对应：
	//   App::where([['state','=',1],['type','=',2]])
	//       ->field('id,name,logo,type,flag')->order('id','desc')
	//       ->limit($offset,$length)->select()->toArray()
	ListApps(ctx context.Context, offset, limit int) ([]OpenAppRow, error)
	// CountApps 对应 `App::where($this->queryWhere())->count()`
	CountApps(ctx context.Context) (int64, error)
	// FindApp 对应：
	//   App::where('id', $appId)->field('id,name,logo,type,fee_rate,flag')->findOrEmpty()
	FindApp(ctx context.Context, appID int64) (*OpenAppDetailRow, bool, error)
}

// OpenAppRow 对应 field('id,name,logo,type,flag') —— 只有这 5 列。
type OpenAppRow struct {
	ID   int64
	Name string
	// ⚠️ 库里是相对路径；lists() 会（且只有 lists 会）经 get_image_url 拼域名
	Logo string
	Type int32
	Flag string
}

// OpenAppDetailRow 对应 field('id, name, logo, type, fee_rate, flag')。
//
// ⚠️ 比 lists() 多了 `fee_rate`，**少了 `state`**。
type OpenAppDetailRow struct {
	ID      int64
	Name    string
	Logo    string
	Type    int32
	FeeRate string
	Flag    string
}

// OpenAppSelect = 'id,name,logo,type,flag'（lists 用）
const OpenAppSelect = "id,name,logo,type,flag"

// OpenAppDetailSelect = 'id, name, logo, type, fee_rate, flag'（detail 用）
const OpenAppDetailSelect = "id,name,logo,type,fee_rate,flag"

// 查询条件（lists 与 count 共用）：
//
//	[['state','=',1], ['type','=',2]]
//
// ⚠️ `type = 2` 表示"三方平台"（x_app.type：1=本地系统，2=三方）。
// ⚠️ 两个条件都硬编码，且**必须 list 与 count 一致**，否则分页错位。
const (
	OpenAppStateEnabled int32 = 1
	OpenAppTypeThird    int32 = 2
)

// List 对应 `dataLists(new AppLists())`。
//
// ⚠️ `lists()` 里对 `logo` 做了处理：
//
//		$list['logo'] = !empty($list['logo']) ? get_image_url($list['logo']) : '';
//
//	   **空值会被换成空串**（不是 null）。域名由 handler 传入。
func (uc *OpenAppUsecase) List(ctx context.Context, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountApps(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListApps(ctx, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// Detail 对应 `(new AppLists())->detail()`。
//
// 原文：
//
//	App::where('id', $app_id ?: $this->params['app_id'])
//	    ->field('id, name, logo, type, fee_rate, flag')
//	    ->findOrEmpty()->toArray();
//
// ⚠️⚠️ **detail 不对 logo 做处理**（lists 会）—— 这里返回的是库里的**相对路径**。
//
//	原项目的不一致，照搬，不要"顺手对齐"。
//
// ⚠️ 查不到时 `findOrEmpty()->toArray()` 返回**空数组 []**，不是 null ——
//
//	所以返回 (nil, false) 时 handler 要输出 `[]`，不能输出 `{}`。
func (uc *OpenAppUsecase) Detail(ctx context.Context, appID int64) (*OpenAppDetailRow, bool, error) {
	return uc.repo.FindApp(ctx, appID)
}
