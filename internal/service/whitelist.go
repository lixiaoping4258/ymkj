package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// WhitelistService 对应 app/api/controller/v1/whitelist/WhitelistController.php。
type WhitelistService struct {
	v1.UnimplementedWhitelistServiceServer
	wl *biz.WhitelistUsecase
}

func NewWhitelistService(wl *biz.WhitelistUsecase) *WhitelistService {
	return &WhitelistService{wl: wl}
}

// CheckWhitelistItem 对应 WhitelistController::check（GET /v1/whitelist/check?item_code=x）。
//
// 原实现：
//
//	$itemCode = $this->request->get('item_code', '');
//	if (empty($itemCode)) { return $this->fail('缺少限制项标识(item_code)'); }
//	$hasPermission = WhitelistLogic::checkPermission($this->userId, $itemCode);
//	return $this->data(['has_permission' => $hasPermission, 'item_code' => $itemCode]);
//
// ⚠️ `empty($itemCode)` —— 空串与 '0' 都算"缺少"。这里按空串处理
//
//	（item_code 是英文标识，'0' 不是合法值）。
//
// ⚠️ `has_permission` 为 **true 表示受限（被禁止）** —— 语义容易反，已在 proto 注释写明。
func (s *WhitelistService) CheckWhitelistItem(
	ctx context.Context, req *v1.CheckWhitelistItemRequest,
) (*v1.RawData, error) {
	itemCode := req.GetItemCode()
	if itemCode == "" {
		return nil, bizFail(biz.NewBizError("缺少限制项标识(item_code)"))
	}
	userID := biz.UserIDFromContext(ctx)
	has, err := s.wl.CheckPermission(ctx, userID, itemCode)
	if err != nil {
		return nil, err
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"has_permission": has,
		"item_code":      itemCode,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetWhitelistPermissions 对应 WhitelistController::permissions。
//
// 原实现：
//
//	$permissions = WhitelistLogic::getUserPermissions($this->userId);
//	return $this->data(['permissions' => $permissions]);
func (s *WhitelistService) GetWhitelistPermissions(
	ctx context.Context, _ *v1.GetWhitelistPermissionsRequest,
) (*v1.RawData, error) {
	userID := biz.UserIDFromContext(ctx)
	perms, err := s.wl.UserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{"permissions": perms})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// GetWhitelistGroup 对应 WhitelistController::group。
//
// 原实现：
//
//	$group = WhitelistLogic::getUserGroup($this->userId);
//	return $this->data(['group' => $group]);     // $group 可能是 null
func (s *WhitelistService) GetWhitelistGroup(
	ctx context.Context, _ *v1.GetWhitelistGroupRequest,
) (*v1.RawData, error) {
	userID := biz.UserIDFromContext(ctx)
	res, err := s.wl.GetUserGroup(ctx, userID)
	if err != nil {
		return nil, err
	}
	var group any
	if res.Group != nil {
		group = res.Group
	}
	// nil -> JSON null（不是 {}）
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{"group": group})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}
