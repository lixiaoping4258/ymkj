package biz

import "context"

// BalanceQueryUsecase 对应 app/api/logic/wallet/BalanceWalletLogic.php::queryBalance。
type BalanceQueryUsecase struct {
	repo BalanceQueryRepo
}

func NewBalanceQueryUsecase(repo BalanceQueryRepo) *BalanceQueryUsecase {
	return &BalanceQueryUsecase{repo: repo}
}

// BalanceQueryRepo 渠道余额查询。
type BalanceQueryRepo interface {
	// FindPayConfigByWayID 对应：
	//
	//	PayWay::hasWhere('payConfig',
	//	        ['PayWay.id'=>$channel, 'PayWay.status'=>1],
	//	        'PayConfig.id,PayConfig.pay_way')->find();
	//
	// ⚠️ 注意条件里的 `PayWay.id` —— 传进来的 `channel` 是**支付方式 ID**，
	//    不是 scene（与 payment/channel 的 scene 不是一回事）。
	// 返回 (payWay 值, 是否找到, 错误)。
	FindPayConfigByWayID(ctx context.Context, channel int64) (payWay int32, found bool, err error)

	// UserMoney 对应 `User::where(['id'=>$userId,'is_disable'=>0])->value('user_money')`。
	//
	// ⚠️ 用户不存在或 is_disable != 0 时，ThinkPHP 的 value() 返回 **null**，
	//    所以这里第三个返回值区分"查到值"与"没查到"。
	UserMoney(ctx context.Context, userID uint32) (amount string, found bool, err error)
}

// 逐字对应原文的 switch 分支
const (
	// 积分、余额 -> 查 user_money
	PayWayBalance int32 = 1
	// 微信 -> 恒 0
	PayWayWechat int32 = 2
	// （原文注释缺失）-> 恒 0
	PayWayThird int32 = 3
	// 未识别的 pay_way -> **-1**
	PayWayUnknownResult = -1
)

// QueryBalance 逐字对应 BalanceWalletLogic::queryBalance($userId, $channel)。
//
// 原文：
//
//	$condition = ['PayWay.id' => $channel, 'PayWay.status' => 1];
//	$record = PayWay::hasWhere('payConfig', $condition, 'PayConfig.id,PayConfig.pay_way')->find();
//	if (!$record) { return 0; }                      // 没有余额
//	switch ($record->pay_way) {
//	    case 1:   // 积分、余额
//	        return User::where(['id'=>$userId,'is_disable'=>0])->value('user_money');
//	    case 2:   return 0;                          // 微信
//	    case 3:   return 0;
//	    default:  return -1;                         // ⚠️ 未识别的 pay_way 返回 -1
//	}
//
// 控制器：`return $this->data(['balance' => $value]);`
//
// ⚠️⚠️ 返回值的**类型是变化的**，Go 侧必须用 any 承接：
//   - 渠道不存在            -> **int 0**
//   - pay_way = 2/3        -> **int 0**
//   - pay_way = 1 且用户存在 -> **string**（decimal 列，如 "123.45"）
//   - pay_way = 1 但用户不存在/被禁用 -> **null**
//   - pay_way 是其它值      -> **int -1**
//
// 用 int64/string 单一类型都会丢信息。
func (uc *BalanceQueryUsecase) QueryBalance(ctx context.Context, userID uint32, channel int64) (any, error) {
	payWay, found, err := uc.repo.FindPayConfigByWayID(ctx, channel)
	if err != nil {
		return nil, err
	}
	if !found {
		// 对应 `if(!$record) { return 0; }` —— **int 0**
		return 0, nil
	}
	switch payWay {
	case PayWayBalance:
		amount, ok, err := uc.repo.UserMoney(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !ok {
			// value() 查不到 -> null
			return nil, nil
		}
		return amount, nil
	case PayWayWechat, PayWayThird:
		return 0, nil
	default:
		return PayWayUnknownResult, nil
	}
}
