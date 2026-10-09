package biz

import "context"

// PlatformUsecase 对应 app/api/logic/open/OpenAppLogic.php 的 getThirdAppsList。
//
// ⚠️ 只有这一个方法，放在独立的 usecase 里而不是塞进 ConfigUsecase ——
// 原文的调用方是 PlatformController，与配置无关。
type PlatformUsecase struct {
	repo PlatformRepo
}

func NewPlatformUsecase(repo PlatformRepo) *PlatformUsecase {
	return &PlatformUsecase{repo: repo}
}

// PlatformRepo 第三方平台查询端口。
type PlatformRepo interface {
	// ListThirdApps 对应：
	//
	//	App::where(['state' => $state, 'type' => 2])
	//	    ->field('name,flag')->select()->toArray();
	//
	// ⚠️ type = 2 是**硬编码**在方法体里的，不是参数。
	ListThirdApps(ctx context.Context, state int32, appType int32) ([]PlatformItem, error)
}

// PlatformItem 对应 field('name,flag') 的两列。
type PlatformItem struct {
	Name string
	Flag string
}

// ThirdAppStateEnabled 对应 getThirdAppsList 的 $state 默认值 1。
const ThirdAppStateEnabled = 1

// ThirdAppType 对应方法体里硬编码的 type = 2。
const ThirdAppType = 2

// ListThirdApps 逐字对应 OpenAppLogic::getThirdAppsList()（控制器不传参，用默认值）。
func (uc *PlatformUsecase) ListThirdApps(ctx context.Context) ([]PlatformItem, error) {
	return uc.repo.ListThirdApps(ctx, ThirdAppStateEnabled, ThirdAppType)
}

// ProtocolResult 对应 IndexLogic::getPolicyByType 的返回值。
type ProtocolResult struct {
	Title   string
	Content string
}
