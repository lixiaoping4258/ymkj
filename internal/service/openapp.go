package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
)

// OpenAppService 对应 app/api/controller/v1/open/OpenAppController.php。
type OpenAppService struct {
	v1.UnimplementedOpenAppServiceServer
	app    *biz.OpenAppUsecase
	authMy *biz.AuthMyUserUsecase
	authU  *biz.AuthUserUsecase
	authUD *biz.AuthUserDetailUsecase
}

func NewOpenAppService(app *biz.OpenAppUsecase, authMy *biz.AuthMyUserUsecase, authU *biz.AuthUserUsecase, authUD *biz.AuthUserDetailUsecase) *OpenAppService {
	return &OpenAppService{app: app, authMy: authMy, authU: authU, authUD: authUD}
}

// GetAppList 对应 OpenAppController::appList（GET /v1/open/appList）。
//
// 原实现：`return $this->dataLists(new AppLists());`
func (s *OpenAppService) GetAppList(
	ctx context.Context, _ *v1.GetAppListRequest,
) (*v1.RawData, error) {
	p := biz.ParsePageParams(httpx.QueryFrom(ctx))
	ld, err := s.app.List(ctx, p)
	if err != nil {
		return nil, err
	}
	domain := domainFrom(ctx)

	// ⚠️ logo 的处理只在这里做：
	//    !empty($logo) ? get_image_url($logo) : ''
	//    **空值换成空串**（不是 null，也不是保持原值）。
	items, _ := ld.Lists.([]biz.OpenAppRow)
	rows := make([]map[string]any, 0)
	for _, x := range items {
		logo := ""
		if x.Logo != "" {
			logo = joinFileURLByDomain(domain, x.Logo)
		}
		rows = append(rows, map[string]any{
			"id":   x.ID,
			"name": x.Name,
			"logo": logo,
			"type": x.Type,
			"flag": x.Flag,
		})
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"lists":     rows,
		"count":     ld.Count,
		"page_no":   ld.PageNo,
		"page_size": ld.PageSize,
		"extend":    ld.Extend,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetAppDetail 对应 OpenAppController::appDetail（GET /v1/open/appDetail?app_id=N）。
//
// 原实现：`return $this->data((new AppLists())->detail());`
func (s *OpenAppService) GetAppDetail(
	ctx context.Context, req *v1.GetAppDetailRequest,
) (*v1.RawData, error) {
	var appID int64
	if req.AppId != nil {
		appID = req.AppId.Value
	}
	row, found, err := s.app.Detail(ctx, appID)
	if err != nil {
		return nil, err
	}
	if !found || row == nil {
		// `findOrEmpty()->toArray()` 返回**空数组**
		return &v1.RawData{Json: []byte("[]")}, nil
	}
	// ⚠️ 这里**故意不处理 logo** —— detail 返回库里的相对路径（与 lists 不一致，原文如此）
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"id":       row.ID,
		"name":     row.Name,
		"logo":     row.Logo,
		"type":     row.Type,
		"fee_rate": row.FeeRate,
		"flag":     row.Flag,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetAuthUrl 对应 OpenAppController::authUrl（GET /v1/open/authUrl）。
//
// 原实现：
//
//	$data = (new AuthUrlValidate())->get()->goCheck();
//	$result = OpenAppLogic::authUrl($data['app_id'], $data['redirect_url']);
//	if (false === $result) { return $this->fail(OpenAppLogic::getError()); }
//	return $this->data($result);
//
// 校验文案（AuthUrlValidate）：
//
//	'app_id.require'       => '一级平台ID不能为空',
//	'redirect_url.require' => '跳转地址不能为空',
func (s *OpenAppService) GetAuthUrl(
	ctx context.Context, req *v1.GetAuthUrlRequest,
) (*v1.RawData, error) {
	if req.AppId == nil {
		return nil, bizFail(biz.NewBizError("一级平台ID不能为空"))
	}
	if req.GetRedirectUrl() == "" {
		return nil, bizFail(biz.NewBizError("跳转地址不能为空"))
	}
	res, err := s.app.AuthURL(ctx, req.AppId.Value, req.GetRedirectUrl())
	if err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{"auth_url": res.AuthURL})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetAuthMyUserList 对应 OpenAppController::authMyUserList（GET /v1/open/authMyUserList）。
//
// 原实现：`return $this->dataLists(new AuthMyUserLists());`
//
// ⚠️ mobile 的掩码在 biz 层已做（MaskMobile，有单测对拍真实 PHP）。
func (s *OpenAppService) GetAuthMyUserList(
	ctx context.Context, _ *v1.GetAuthMyUserListRequest,
) (*v1.RawData, error) {
	p := biz.ParsePageParams(httpx.QueryFrom(ctx))
	userID := biz.UserIDFromContext(ctx)
	ld, err := s.authMy.List(ctx, uint32(userID), p)
	if err != nil {
		return nil, err
	}
	items, _ := ld.Lists.([]biz.AuthMyUserRow)
	rows := make([]map[string]any, 0, len(items))
	for _, x := range items {
		rows = append(rows, map[string]any{
			"id":           x.ID,
			"app_id":       x.AppID,
			"app_nickname": x.AppNickname,
			"app_avatar":   x.AppAvatar,
			"expire":       x.Expire,
			"app_name":     x.AppName,
			"mobile":       x.Mobile, // 已掩码
		})
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"lists":     rows,
		"count":     ld.Count,
		"page_no":   ld.PageNo,
		"page_size": ld.PageSize,
		"extend":    ld.Extend,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetAuthUserList 对应 OpenAppController::authUserList（GET /v1/open/authUserList?app_id=N）。
//
// 原实现：
//
//	(new AuthUserListValidate())->get()->goCheck();      // app_id => require
//	return $this->dataLists(new AppUserLists());
func (s *OpenAppService) GetAuthUserList(
	ctx context.Context, req *v1.GetAuthUserListRequest,
) (*v1.RawData, error) {
	if req.AppId == nil {
		return nil, bizFail(biz.NewBizError("一级平台ID不能为空"))
	}
	p := biz.ParsePageParams(httpx.QueryFrom(ctx))
	userID := biz.UserIDFromContext(ctx)
	ld, err := s.authU.List(ctx, uint32(userID), req.AppId.Value, p)
	if err != nil {
		return nil, err
	}
	items, _ := ld.Lists.([]biz.AuthUserItem)
	rows := make([]map[string]any, 0, len(items))
	for _, x := range items {
		row := map[string]any{
			"id":           x.ID,
			"app_id":       x.AppID,
			"app_nickname": x.AppNickname,
			"app_avatar":   x.AppAvatar,
			"start":        x.Start,
			"expire":       x.Expire,
			"mobile":       x.Mobile,
			// ⚠️ 查不到时是 **null**（PHP 的 Undefined array key 结果）
			"app_info": appInfoAny(x.AppInfo),
		}
		rows = append(rows, row)
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"lists":     rows,
		"count":     ld.Count,
		"page_no":   ld.PageNo,
		"page_size": ld.PageSize,
		"extend":    ld.Extend,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// appInfoAny 把嵌套的 app_info 转成 map；nil -> nil（序列化成 JSON null）。
//
// ⚠️ 这里**不处理 logo**（AppLists::listsWithQuery 不处理，只有 lists() 处理）。
func appInfoAny(a *biz.AppInfoRow) any {
	if a == nil {
		return nil
	}
	return map[string]any{
		"id":       a.ID,
		"name":     a.Name,
		"state":    a.State,
		"logo":     a.Logo,
		"type":     a.Type,
		"fee_rate": a.FeeRate,
		"flag":     a.Flag,
	}
}

// GetAuthUserDetail 对应 OpenAppController::authUserDetail（GET /v1/open/authUserDetail）。
//
// 原实现：
//
//	(new AuthUserListValidate())->get()->goCheck();     // app_id => require
//	return $this->data((new AppUserLists())->detail());
//
// ⚠️⚠️ **校验的是 `app_id`，查询用的是 `app_auth_id`** —— 原项目的不一致。
//
//	所以 app_id 必须存在（否则校验失败），而 app_auth_id 决定能否取到授权记录。
func (s *OpenAppService) GetAuthUserDetail(
	ctx context.Context, req *v1.GetAuthUserDetailRequest,
) (*v1.RawData, error) {
	if req.AppId == nil {
		return nil, bizFail(biz.NewBizError("一级平台ID不能为空"))
	}
	var authID int64
	if req.AppAuthId != nil {
		authID = req.AppAuthId.Value
	}
	res, err := s.authUD.Detail(ctx, authID, req.AppId.Value)
	if err != nil {
		return nil, err
	}
	// 以 $appAuth 为基底（可能是空 map），再塞 app_info
	out := make(map[string]any, len(res.Auth)+1)
	for k, v := range res.Auth {
		out[k] = v
	}
	if res.AppInfo == nil {
		// `findOrEmpty()->toArray()` 是**空数组**
		out["app_info"] = []any{}
	} else {
		out["app_info"] = map[string]any{
			"id":       res.AppInfo.ID,
			"name":     res.AppInfo.Name,
			"logo":     res.AppInfo.Logo, // ⚠️ 不处理（相对路径）
			"type":     res.AppInfo.Type,
			"fee_rate": res.AppInfo.FeeRate,
			"flag":     res.AppInfo.Flag,
		}
	}
	body, merr := biz.MarshalNoHTMLEscape(out)
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}
