package biz

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// Cache 是 PHP cache() 的最小等价物。
//
// 语义对齐要点：PHP 的 cache($key) 在「键不存在」和「存的是 null」两种情况下
// 都返回 null，而调用方一律用 `!== null` 判空 —— 也就是说**存 null 等于没存**。
// 所以这里的 Get 用 nil 表示未命中，不需要额外的 found 标志。
type Cache interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
}

// 交易时段缓存键，与原项目 TradeConfigService 里的常量逐字一致。
const (
	CacheKeyTradePeriods = "trade:market:periods"
	CacheKeyTradeToday   = "trade:market:todayType"
)

// TradeCalendarRepo 对应 app/common/model/TradeCalendar.php。
type TradeCalendarRepo interface {
	// GetTypeByDate 返回该日期在交易日历表里手动标记的类型。
	// found=false 表示这天没有人工标记，调用方回退到「按周几判断」。
	GetTypeByDate(ctx context.Context, date string) (typeVal int, found bool, err error)
}

// TradePeriod 是一个交易时段。
type TradePeriod struct {
	Start string
	End   string
}

// TradeTimeStatus 是 checkTradeTime() 的完整返回。
type TradeTimeStatus struct {
	TradeSwitch  any // 原: ConfigService::get('trade','tradeSwitch',0)
	IsOpen       int32
	TodayType    string // workday | holiday
	TodayTypeStr string // 工作日 | 节假日
	Periods      []TradePeriod
	Tips         string
}

// TradeConfigUsecase 复刻 app/common/service/TradeConfigService.php。
type TradeConfigUsecase struct {
	config   *ConfigUsecase
	cache    Cache
	calendar TradeCalendarRepo
	loc      *time.Location
	log      *log.Helper
}

func NewTradeConfigUsecase(
	config *ConfigUsecase,
	cache Cache,
	calendar TradeCalendarRepo,
	loc *time.Location,
	logger log.Logger,
) *TradeConfigUsecase {
	return &TradeConfigUsecase{
		config:   config,
		cache:    cache,
		calendar: calendar,
		loc:      loc,
		log:      log.NewHelper(logger),
	}
}

// CheckTradeTime 对应 TradeConfigService::checkTradeTime()。
//
// 注意原实现里 getTodayType() 被调用了三次。因为有缓存（到当天 24 点），
// 实际最多只查一次 DB，这里保持同样的调用结构以便对照。
func (uc *TradeConfigUsecase) CheckTradeTime(ctx context.Context) (*TradeTimeStatus, error) {
	todayType, err := uc.getTodayType(ctx)
	if err != nil {
		return nil, err
	}
	periods, err := uc.getPeriods(ctx, todayType)
	if err != nil {
		return nil, err
	}

	tradeSwitch, err := uc.config.GetDefault(ctx, "trade", "tradeSwitch", 0)
	if err != nil {
		return nil, err
	}

	st := &TradeTimeStatus{
		TradeSwitch: tradeSwitch,
		TodayType:   todayType,
		Periods:     periods,
	}
	if todayType == "workday" {
		st.TodayTypeStr = "工作日"
	} else {
		st.TodayTypeStr = "节假日"
	}

	isOpen, err := uc.IsTradingOpen(ctx)
	if err != nil {
		return nil, err
	}
	st.IsOpen = isOpen

	// ---- tips 文案 ----
	// 逐行对照原实现：逗号只在 start != end 的分支里补，且只补在非末项后面。
	tradeTips, err := uc.config.Get(ctx, "trade", "tradeTips")
	if err != nil {
		return nil, err
	}
	if !phpEmpty(tradeTips) {
		st.Tips = toStr(tradeTips)
		return st, nil
	}

	var b strings.Builder
	b.WriteString("兑换时间段为:")
	if len(periods) > 0 {
		for k, p := range periods {
			if p.Start == p.End {
				b.WriteString(p.Start)
			} else {
				b.WriteString(p.Start)
				b.WriteString("-")
				b.WriteString(p.End)
				if k < len(periods)-1 {
					b.WriteString(",")
				}
			}
		}
		b.WriteString(";周末及节假日暂停兑换")
	} else {
		b.Reset()
		b.WriteString("当前未配置交易时段(")
		b.WriteString(st.TodayTypeStr)
		b.WriteString(")")
	}
	st.Tips = b.String()
	return st, nil
}

// IsTradingOpen 对应 TradeConfigService::isTradingOpen()。
//
// ⚠️ 原实现的注释块写明了「1. 全局总开关（0=开启, 1=关闭）」这一步，
// **但代码里从未读取 tradeSwitch** —— 也就是说 isOpen 完全忽略了全局交易开关，
// 只要在时段内就返回 1。
//
// 这里**刻意保留该行为**，保证迁移期新旧系统判断一致。
// 如果这是 bug 需要修，请在业务确认后单独提一个改动，不要顺手改掉。
func (uc *TradeConfigUsecase) IsTradingOpen(ctx context.Context) (int32, error) {
	todayType, err := uc.getTodayType(ctx)
	if err != nil {
		return 0, err
	}
	periods, err := uc.getPeriods(ctx, todayType)
	if err != nil {
		return 0, err
	}
	if len(periods) == 0 {
		return 0, nil
	}
	now := uc.now().Format("15:04")
	for _, p := range periods {
		// PHP 是字符串比较：$now >= $start && $now < $end
		if now >= p.Start && now < p.End {
			return 1, nil
		}
	}
	return 0, nil
}

// getTodayType 对应 TradeConfigService::getTodayType()。
//
// 优先级：交易日历表人工标记 > 按周几判断。缓存到当天 24 点。
func (uc *TradeConfigUsecase) getTodayType(ctx context.Context) (string, error) {
	today := uc.now().Format("2006-01-02")
	cacheKey := CacheKeyTradeToday + ":" + today

	if v, err := uc.cache.Get(ctx, cacheKey); err != nil {
		// 缓存故障不能影响主流程，降级到 DB（PHP 缓存挂了同理）
		uc.log.WithContext(ctx).Warnf("读取 todayType 缓存失败: %v", err)
	} else if v != nil {
		if s, ok := v.(string); ok {
			return s, nil
		}
	}

	// 先查日历表是否手动标记
	typeVal, found, err := uc.calendar.GetTypeByDate(ctx, today)
	if err != nil {
		return "", err
	}

	var result string
	if found {
		// PHP: $record->type == 1 ? 'workday' : 'holiday'
		if typeVal == 1 {
			result = "workday"
		} else {
			result = "holiday"
		}
	} else {
		// 自然日历：周一到周五工作日，周六日节假日
		wd := int(uc.now().Weekday()) // Go: 周日=0
		if wd == 0 {
			wd = 7 // 对齐 PHP date('N') 的 1..7
		}
		if wd <= 5 {
			result = "workday"
		} else {
			result = "holiday"
		}
	}

	if err := uc.cache.Set(ctx, cacheKey, result, uc.secondsUntilMidnight()); err != nil {
		uc.log.WithContext(ctx).Warnf("写入 todayType 缓存失败: %v", err)
	}
	return result, nil
}

// getPeriods 对应 TradeConfigService::getPeriods()，缓存 300 秒。
func (uc *TradeConfigUsecase) getPeriods(ctx context.Context, dayType string) ([]TradePeriod, error) {
	if v, err := uc.cache.Get(ctx, CacheKeyTradePeriods); err != nil {
		uc.log.WithContext(ctx).Warnf("读取 periods 缓存失败: %v", err)
	} else if v != nil {
		if m, ok := v.(map[string]any); ok {
			return toPeriods(m[dayType]), nil
		}
	}

	workday, err := uc.config.GetDefault(ctx, "trade", "workdayPeriods", []any{})
	if err != nil {
		return nil, err
	}
	holiday, err := uc.config.GetDefault(ctx, "trade", "holidayPeriods", []any{})
	if err != nil {
		return nil, err
	}
	cfg := map[string]any{"workday": workday, "holiday": holiday}
	if err := uc.cache.Set(ctx, CacheKeyTradePeriods, cfg, 300*time.Second); err != nil {
		uc.log.WithContext(ctx).Warnf("写入 periods 缓存失败: %v", err)
	}
	return toPeriods(cfg[dayType]), nil
}

// secondsUntilMidnight 对应 strtotime('tomorrow') - time()。
func (uc *TradeConfigUsecase) secondsUntilMidnight() time.Duration {
	n := uc.now()
	tomorrow := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, uc.loc).AddDate(0, 0, 1)
	// 至少留 1 秒，避免 0 被当成「不过期」
	if d := tomorrow.Sub(n); d > time.Second {
		return d
	}
	return time.Second
}

func (uc *TradeConfigUsecase) now() time.Time {
	if uc.loc == nil {
		return time.Now()
	}
	return time.Now().In(uc.loc)
}

// toPeriods 把 ConfigService 返回的任意结构转成 []TradePeriod。
//
// 原实现直接下标访问 $p['start']，缺字段时 PHP 8 会告警并按 null 比较。
// 这里取零值 ""，行为等价且不会崩。
func toPeriods(v any) []TradePeriod {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]TradePeriod, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, TradePeriod{
			Start: toStr(m["start"]),
			End:   toStr(m["end"]),
		})
	}
	return out
}

// phpEmpty 对应 PHP 的 empty()。
func phpEmpty(v any) bool {
	return !PhpTruthy(v)
}

// toStr 把任意值转成 PHP 风格的字符串。
func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "1"
		}
		return ""
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}
