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
	app *biz.OpenAppUsecase
}

func NewOpenAppService(app *biz.OpenAppUsecase) *OpenAppService {
	return &OpenAppService{app: app}
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
