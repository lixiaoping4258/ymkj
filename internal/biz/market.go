package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/market/PurchaseController::payWay / checkExchange
//   app/api/logic/warehouse/GoodsLogic::checkExchange

// x_market_purchase.state 的取值（原项目 MarketPurchaseEnum）。
//
// 注意有两套状态：数字的 ADMIN_STATE_* 给后台看，
// 字符串的 STATE_* 用在业务判断上。这里只用到 STATE_WANTED。
const (
	MarketPurchaseStateOffline  = "OFFLINE"  // 下架
	MarketPurchaseStateWanted   = "WANTED"   // 兑换中
	MarketPurchaseStateComplete = "COMPLETE" // 完成
	MarketPurchaseStateExpire   = "EXPIRE"   // 超时
)

// PayWay 是支付方式。
type PayWay struct {
	PayWay  int32
	PayName string
}

// payWays 与原实现一样是**硬编码**的，不查库。
//
// 原代码：
//
//	public function payWay(): Json {
//	    $data = [
//	        ['pay_way' => 1, 'pay_name' => '茶交所'],
//	        ['pay_way' => 2, 'pay_name' => '陶交所'],
//	    ];
//	    return $this->data($data);
//	}
//
// 保持硬编码是刻意的：它的作用只是告诉前端 pay_way 的取值含义。
// 改成查 x_pay_way 表会让行为变化（那张表的内容与这里的语义不一定一致）。
var payWays = []PayWay{
	{PayWay: 1, PayName: "茶交所"},
	{PayWay: 2, PayName: "陶交所"},
}

// MarketPurchaseRepo 市场兑换单数据访问。
type MarketPurchaseRepo interface {
	// CountByUserAndState 对应 MarketPurchase::where(['user_id'=>..,'state'=>..])->count()
	CountByUserAndState(ctx context.Context, userID uint64, state string) (int64, error)
}

// Locker 是分布式锁端口。
//
// 原项目对应 app/common/service/RedisLockService.php。
//
// ⚠️ 与缓存相反：锁的键**必须与 PHP 共用**，不能加隔离前缀。
// 因为锁值只是个随机 token，没有任何序列化格式，两边完全兼容；
// 而共用才能保证用户不会在 PHP 和 Go 上各提交一次（防重复点击会失效）。
// 缓存则必须隔离 —— 那边存的是 ThinkPHP 的序列化结构。
type Locker interface {
	// TryLock 非阻塞抢锁。ok=false 表示已被别人持有。
	// 返回的 token 用于 Unlock 时校验持有者。
	TryLock(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error)
	// Unlock 释放锁（仅当 token 匹配时才释放）。
	Unlock(ctx context.Context, key, token string) error
}

// MarketUsecase 市场域用例。
type MarketUsecase struct {
	purchase MarketPurchaseRepo
	trade    *TradeConfigUsecase
	locker   Locker
	log      *log.Helper
}

func NewMarketUsecase(
	purchase MarketPurchaseRepo,
	trade *TradeConfigUsecase,
	locker Locker,
	logger log.Logger,
) *MarketUsecase {
	return &MarketUsecase{
		purchase: purchase,
		trade:    trade,
		locker:   locker,
		log:      log.NewHelper(logger),
	}
}

// PayWay 对应 PurchaseController::payWay。
func (uc *MarketUsecase) PayWay(_ context.Context) []PayWay {
	out := make([]PayWay, len(payWays))
	copy(out, payWays)
	return out
}

// CheckExchange 对应 GoodsLogic::checkExchange($userId, $params)。
//
// 原实现（注意 $params 完全没被用到）：
//
//	$key = "user:exchange:check:$userId";
//	$tryLock = RedisLockService::tryLock("click:" . $key, 2000);
//	if (!$tryLock) { setError('请勿重复操作'); return false; }   // 2 秒防重复点击
//	$tradeSwitchCheck = TradeConfigService::checkTradeTime();
//	if ($tradeSwitchCheck['tradeSwitch'] == 1 && $tradeSwitchCheck['isOpen'] != 1) {
//	    setError('当前不在兑换时间内，暂无法导入数字艺术品'); return false;
//	}
//	$purchaseNum = MarketPurchase::where(['user_id'=>$userId,'state'=>'WANTED'])->count();
//	if ($purchaseNum > 0) { setError('当前存在兑换单，暂无法导入数字艺术品'); return false; }
//	return [];
//
// ⚠️ 原实现**从头到尾没有解锁**，锁靠 2000ms 过期自动释放。
// 所以它本质是「2 秒防抖」而不是互斥锁，这里保持同样行为（不加 Unlock）。
func (uc *MarketUsecase) CheckExchange(ctx context.Context, userID uint64) error {
	// 键与 PHP 逐字一致，保证跨系统防重复点击有效
	lockKey := fmt.Sprintf("click:user:exchange:check:%d", userID)
	if _, ok, err := uc.locker.TryLock(ctx, lockKey, 2*time.Second); err != nil {
		// 锁服务故障不应该把用户挡在门外 —— 这与原实现一致
		// （PHP 那边 redis 报错会抛异常，但这里降级更稳妥）
		uc.log.WithContext(ctx).Warnf("兑换校验抢锁失败，降级继续: %v", err)
	} else if !ok {
		return NewBizError("请勿重复操作")
	}

	st, err := uc.trade.CheckTradeTime(ctx)
	if err != nil {
		return err
	}
	// PHP 是宽松比较，tradeSwitch 的实际类型不确定（int / "1" / float64）
	if PhpLooseEqualsInt(st.TradeSwitch, 1) && st.IsOpen != 1 {
		return NewBizError("当前不在兑换时间内，暂无法导入数字艺术品")
	}

	n, err := uc.purchase.CountByUserAndState(ctx, userID, MarketPurchaseStateWanted)
	if err != nil {
		return err
	}
	if n > 0 {
		return NewBizError("当前存在兑换单，暂无法导入数字艺术品")
	}
	return nil
}
