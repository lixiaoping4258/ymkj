package biz

import (
	"context"
	"strconv"
)

// AuthUserDetailUsecase 对应 `(new AppUserLists())->detail()`。
type AuthUserDetailUsecase struct {
	repo AuthUserDetailRepo
}

func NewAuthUserDetailUsecase(repo AuthUserDetailRepo) *AuthUserDetailUsecase {
	return &AuthUserDetailUsecase{repo: repo}
}

// AuthUserDetailRepo detail() 的两段查询。
type AuthUserDetailRepo interface {
	// FindAuthByID 对应 `AppAuth::where('id', $app_auth_id)->findOrEmpty()->toArray()`。
	//
	// ⚠️ 真实 SQL 是 **`SELECT *`**（不是字段清单）：
	//    SELECT * FROM `x_app_auth` WHERE ( `id` = ? ) AND `x_app_auth`.`delete_time` IS NULL LIMIT 1
	//
	// ⚠️🔴 因为 `AppAuth` 没有 `$hidden`，`toArray()` 会返回**全部列** ——
	//    包括 `token`（三方授权凭据）。**这是原项目的凭据外泄面**，照搬但已记录。
	FindAuthByID(ctx context.Context, id int64) (map[string]any, bool, error)

	// FindAppDetail 对应 `AppLists::detail($appId)`：
	//   App::where('id', $appId)->field('id, name, logo, type, fee_rate, flag')->findOrEmpty()->toArray()
	//
	// ⚠️ 6 列；**不处理 logo**（返回库里的相对路径）。
	FindAppDetail(ctx context.Context, appID int64) (*AppInfoRow, bool, error)
}

// AuthUserDetailResult 对应 detail() 的返回：
// `$appAuth`（可能是空数组）+ `app_info` 键。
type AuthUserDetailResult struct {
	Auth map[string]any
	// AppInfo 可能为 nil -> 输出 `[]`（PHP 的 findOrEmpty()->toArray() 空数组）
	AppInfo *AppInfoRow
}

// Detail 逐字对应 AppUserLists::detail()。
//
// 原文：
//
//	$appAuth = AppAuth::where('id', $this->params['app_auth_id'])->findOrEmpty()->toArray();
//	$app = (new AppLists())->detail($appAuth['app_id']);
//	$appAuth['app_info'] = $app;
//	return $appAuth;
//
// ⚠️⚠️ **参数名不一致（原项目如此）**：控制器用 `AuthUserListValidate` 校验的是
//
//	**`app_id`**（require），而这里读的是 **`app_auth_id`**。
//	-> 请求必须**同时带 app_id 和 app_auth_id** 才能正常取到授权记录。
//
// ⚠️ 当 `app_auth_id` 不存在（或记录已被软删除）时：
//
//	  `findOrEmpty()->toArray()` 返回 **`[]`**
//	  -> `$appAuth['app_id']` 触发 Undefined array key，值为 **null**
//	  -> `AppLists::detail(null)` 里 `$app_id ?: $this->params['app_id']`
//	     回退到**校验过的 `app_id` 参数**
//	  -> 于是响应变成 `{"app_info": {...}}`（没有其它键）
//	实测确认：空数组取键得 null；`null ?: 123` 得 123。
//
// ⚠️ 注意 `app_info` 是**用 `app_id` 查的**（不是用找到的 `$appAuth['app_id']`）——
//
//	正常路径下两者相同；异常路径下回退到入参。
func (uc *AuthUserDetailUsecase) Detail(
	ctx context.Context, appAuthID, fallbackAppID int64,
) (*AuthUserDetailResult, error) {
	auth, found, err := uc.repo.FindAuthByID(ctx, appAuthID)
	if err != nil {
		return nil, err
	}
	if !found || auth == nil {
		// `findOrEmpty()->toArray()` -> []，取 ['app_id'] -> null -> 回退到入参
		auth = map[string]any{}
	}

	// `$app_id ?: $this->params['app_id']` —— PHP 的 ?: 对 0/''/null 都回退
	lookup := fallbackAppID
	if v, ok := auth["app_id"]; ok && v != nil {
		if n, ok2 := toInt64Any(v); ok2 && n != 0 {
			lookup = n
		}
	}

	app, appFound, err := uc.repo.FindAppDetail(ctx, lookup)
	if err != nil {
		return nil, err
	}
	if !appFound {
		app = nil
	}
	return &AuthUserDetailResult{Auth: auth, AppInfo: app}, nil
}

// toInt64Any 把 any 里的整数转成 int64（PHP 的数组值可能是 int 或数字字符串）。
func toInt64Any(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int32:
		return int64(x), true
	case int:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return int64(x), true
	case string:
		n, err := strconv.ParseInt(x, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
