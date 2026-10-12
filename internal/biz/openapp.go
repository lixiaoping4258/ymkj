package biz

import (
	"context"
	"errors"
)

// OpenAppUsecase 对应 app/api/lists/open/AppLists.php。
type OpenAppUsecase struct {
	repo     OpenAppRepo
	authRepo OpenAppAuthRepo
}

func NewOpenAppUsecase(repo OpenAppRepo, authRepo OpenAppAuthRepo) *OpenAppUsecase {
	return &OpenAppUsecase{repo: repo, authRepo: authRepo}
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

/* ------------------------------------------------- authUrl */

// OpenAppAuthRepo authUrl 的查询。
type OpenAppAuthRepo interface {
	// FindAuthURL 对应：
	//   App::where('state','=',1)->where('id','=',$appId)->find()
	//
	// ⚠️ 只筛 `state = 1`，**不筛 type** —— 与 AppLists 的 `state=1 AND type=2` 不同，
	//    所以 type=1 的本地系统（如 id=2）也能查到。原文如此。
	FindAuthURL(ctx context.Context, appID int64) (authURL string, found bool, err error)
}

// 逐字对应原文的两条错误
var (
	ErrOpenAppNotFound  = errors.New("未获取到一级平台信息")
	ErrOpenAppNoAuthURL = errors.New("未获取到一级平台信息授权地址")
)

// OpenAppAuthResult 对应 `['auth_url' => $url]`。
type OpenAppAuthResult struct {
	AuthURL string
}

// AuthURL 逐字对应 OpenAppLogic::authUrl($appId, $redirect_url)。
//
// 原文：
//
//	$app = App::where('state','=',1)->where('id','=',$appId)->find();
//	if (!$app) { self::setError('未获取到一级平台信息'); return false; }
//	if (!$app->auth_url) { self::setError('未获取到一级平台信息授权地址'); return false; }
//	$url = sprintf('%s&appId=%s&state=111&redirectUrl=%s', $app->auth_url, $appId, $redirect_url);
//	return ['auth_url' => $url];
//
// ⚠️⚠️ 三处必须逐字照搬：
//  1. **`state=111` 是硬编码的**（不是从配置读的、也不是 app 的某列）。
//  2. `$app->auth_url` 原样拼接（它自带 `?` 或 `&` 由库里决定，
//     代码只是无脑接 `&appId=...`）—— 不要"帮忙"判断该用 ? 还是 &。
//  3. **`$redirect_url` 没有做 URL 编码**。照搬，不加 urlencode。
//
// ⚠️ 判断 `!$app->auth_url` 用的是 PHP 的**宽松空判断** ——
//
//	`''`、`'0'`、null 都会命中。Go 侧用 `== ""` 会漏掉 `'0'` 这一种
//	（auth_url 是 varchar，理论上可能是 '0'）。这里按 PHP 语义处理。
func (uc *OpenAppUsecase) AuthURL(ctx context.Context, appID int64, redirectURL string) (*OpenAppAuthResult, error) {
	authURL, found, err := uc.authRepo.FindAuthURL(ctx, appID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrOpenAppNotFound
	}
	// 对应 `if (!$app->auth_url)` —— PHP 的 empty 语义：'' / '0' / null 都算空
	if authURL == "" || authURL == "0" {
		return nil, ErrOpenAppNoAuthURL
	}
	// sprintf('%s&appId=%s&state=111&redirectUrl=%s', ...)
	return &OpenAppAuthResult{
		AuthURL: authURL + "&appId=" + itoa(appID) + "&state=111&redirectUrl=" + redirectURL,
	}, nil
}
