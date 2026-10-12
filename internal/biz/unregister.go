package biz

import "context"

// UnregisterCheckUsecase 对应 UserLogic::unregisterCheck($userId)。
//
// 注销前的三项检查，返回 {state, errWord[]}：
//
//	state = 1 表示可以注销；只要有一项不通过就是 0，并把原因放进 errWord。
type UnregisterCheckUsecase struct {
	repo UnregisterCheckRepo
}

func NewUnregisterCheckUsecase(repo UnregisterCheckRepo) *UnregisterCheckUsecase {
	return &UnregisterCheckUsecase{repo: repo}
}

// UnregisterCheckRepo 三项检查各自的查询。
type UnregisterCheckRepo interface {
	// CountUnsettledOrders 对应：
	//
	//	MarketPurchase::where('user_id','=',$userId)
	//	    ->where(function ($q) {
	//	        $q->where('state','=',STATE_WANTED)->whereOr('pay_state','=',PAY_PAYING);
	//	    })->count()
	//
	// ⚠️ 注意闭包里的 **AND + OR 组合**：`state='WANTED' OR pay_state='PAYING'`，
	//    整体再与 user_id 做 AND。写成 `user_id=? AND state='WANTED' OR pay_state='PAYING'`
	//    会因为 AND 优先级高于 OR 而变成另一个语义（会串到别的用户）。
	//
	// ⚠️ MarketPurchase 有软删除，ThinkPHP 会隐式加 delete_time IS NULL；
	//    Go 侧必须显式加（这是第 6 轮踩过的坑）。
	CountUnsettledOrders(ctx context.Context, userID uint32) (int64, error)

	// CountBoundPlatformAccounts 对应：
	//   UserAccounts::where(['user_id'=>$userId,'type'=>2])->whereIn('app_id',[3,4])->count()
	CountBoundPlatformAccounts(ctx context.Context, userID uint32) (int64, error)

	// HasNonZeroWallet 对应：
	//   UserWallet::where(['user_id'=>$userId])->field('amount,frozen_amount')->select()
	//   然后逐行判断 amount>0 || frozen_amount>0，**命中一行即 break**。
	//
	// 只需要"有没有"，不需要具体金额，所以用 EXISTS 语义（但结果必须与逐行判断一致）。
	HasNonZeroWallet(ctx context.Context, userID uint32) (bool, error)
}

// 逐字对应原文里的三条提示语
const (
	UnregisterErrUnsettledOrder = "账号内还存在未结清的艺术品信息"
	UnregisterErrPlatformBound  = "平台用户未解绑"
	UnregisterErrWalletNotZero  = "用户积分未清零"
)

// 逐字对应 MarketPurchaseEnum 的两个值
const (
	MktStateWanted = "WANTED"
	MktPayPaying   = "PAYING"
)

// UnregisterCheckResult 对应 `['state' => $state, 'errWord' => $errWord]`。
type UnregisterCheckResult struct {
	// State 为 1 表示三项检查都通过（可以注销）；否则 0
	State int32
	// ErrWord 是不通过的原因列表；全部通过时是**空数组**
	ErrWord []string
}

// UnregisterCheck 逐字对应 UserLogic::unregisterCheck($userId)。
//
// 原文（注意三项检查的**顺序**决定 errWord 的先后）：
//
//	$hasUnsettledOrder = MarketPurchase::where('user_id','=',$userId)
//	    ->where(function($q){ $q->where('state','=',WANTED)->whereOr('pay_state','=',PAYING); })
//	    ->count() > 0;
//	$errWord = []; $state = 1;
//	if ($hasUnsettledOrder) { $errWord[] = '账号内还存在未结清的艺术品信息'; }
//
//	$check = UserAccounts::where(['user_id'=>$userId,'type'=>2])->whereIn('app_id',[3,4])->count();
//	if ($check > 0) { $errWord[] = '平台用户未解绑'; }
//
//	$checkUserWallets = UserWallet::where(['user_id'=>$userId])->field('amount,frozen_amount')->select();
//	foreach ($checkUserWallets as $w) {
//	    if ($w['amount'] > 0 || $w['frozen_amount'] > 0) { $errWord[] = '用户积分未清零'; break; }
//	}
//	if (count($errWord) > 0) { $state = 0; }
//	return ['state'=>$state, 'errWord'=>$errWord];
func (uc *UnregisterCheckUsecase) UnregisterCheck(ctx context.Context, userID uint32) (*UnregisterCheckResult, error) {
	errWord := []string{}

	// ① 未结清订单
	n, err := uc.repo.CountUnsettledOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		errWord = append(errWord, UnregisterErrUnsettledOrder)
	}

	// ② 平台用户未解绑（type=2 且 app_id ∈ {3,4}）
	n2, err := uc.repo.CountBoundPlatformAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n2 > 0 {
		errWord = append(errWord, UnregisterErrPlatformBound)
	}

	// ③ 积分未清零
	has, err := uc.repo.HasNonZeroWallet(ctx, userID)
	if err != nil {
		return nil, err
	}
	if has {
		errWord = append(errWord, UnregisterErrWalletNotZero)
	}

	state := int32(1)
	if len(errWord) > 0 {
		state = 0
	}
	return &UnregisterCheckResult{State: state, ErrWord: errWord}, nil
}
