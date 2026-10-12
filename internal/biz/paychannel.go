package biz

import "context"

// PayChannelUsecase 对应 app/common/logic/PaymentLogic.php::getScenePayWay。
type PayChannelUsecase struct {
	repo PayChannelRepo
}

func NewPayChannelUsecase(repo PayChannelRepo) *PayChannelUsecase {
	return &PayChannelUsecase{repo: repo}
}

// PayChannelRepo 支付方式（两表 INNER JOIN）。
type PayChannelRepo interface {
	// ListScenePayWay 对应：
	//
	//	PayWay::hasWhere('payConfig',
	//	        ['PayWay.scene'=>$scene, 'PayWay.status'=>YesNoEnum::YES],
	//	        'PayWay.id,PayWay.is_default,PayConfig.name,PayConfig.icon,
	//	         PayConfig.remark,PayConfig.need_pass')
	//	    ->orderRaw('PayConfig.sort desc')->select()->toArray();
	ListScenePayWay(ctx context.Context, scene int32) ([]PayChannelRow, error)
}

// PayChannelRow 对应 field() 里的 6 列。
type PayChannelRow struct {
	// PayWay.id
	ID int64
	// PayWay.is_default
	IsDefault int32
	// PayConfig.*
	Name     string
	Icon     string
	Remark   string
	NeedPass int32
}

// PayEnum::SCENE_MNP = 3（微信小程序）
//
// 其余场景：SCENE_H5=1 / SCENE_OA=2 / SCENE_MNP=3 / SCENE_APP=4 / SCENE_PC=5
const PaySceneMnp = 3

// PayWayStatusEnabled 对应 `'PayWay.status' => YesNoEnum::YES`（YES = 1）。
const PayWayStatusEnabled = 1

// GetScenePayWay 逐字对应 PaymentLogic::getScenePayWay($scene)。
//
// ⚠️ `hasWhere('payConfig', ...)` 生成的是 **INNER JOIN**（不是 LEFT JOIN）：
//
//	FROM x_dev_pay_way PayWay
//	INNER JOIN x_dev_pay_config PayConfig ON PayConfig.id = PayWay.pay_config_id
//	WHERE PayWay.scene = ? AND PayWay.status = 1
//	ORDER BY PayConfig.sort DESC
//
// pay_config_id 在 x_dev_pay_config 里不存在的支付方式**不会出现**。
//
// ⚠️ `PayWay` 模型声明了 `protected $name = 'dev_pay_way'` ——
//
//	所以表名是 **x_dev_pay_way**，不是 x_pay_way（按类名猜表名会找不到表）。
//
// ⚠️ **`icon` 需要拼域名**：库里存的是相对路径（如 `/uploads/images/...`），
//
//	而 `PayWay` 模型有 `getIconAttr` -> `FileService::getFileUrl($value)`。
//	ThinkPHP 在 toArray() 时按字段名 `icon` 应用该访问器 ——
//	虽然 field() 写的是 `PayConfig.icon`，结果行里键名仍是 `icon`，
//	所以访问器**会**生效。域名由 handler 传入（对应 request()->domain()）。
func (uc *PayChannelUsecase) GetScenePayWay(ctx context.Context, scene int32) ([]PayChannelRow, error) {
	return uc.repo.ListScenePayWay(ctx, scene)
}
