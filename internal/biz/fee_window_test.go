package biz

import (
	"testing"
	"time"
)

// 这个文件补一个真实存在的验证缺口：**促销费率（outFeeDate）的日期窗口**。
//
// 为什么单独测：线上配置是
//
//	outFeeDate = {"fee":"1","start":"2026-09-24","end":"2026-09-24"}
//
// 生效日只有那一天，所以从实现到现在**从来没被真正触发过**。
// 而这段逻辑的输入是日期字符串，涉及时区解析、补时刻、边界比较 ——
// 任何一处算错，都会让促销费率早一天或晚一天生效。**那是钱。**
//
// 纯函数测试，不需要数据库：把配置里的字面值直接喂进来。

const (
	// 与 x_config 里 trade.outFeeDate 的字面值一致
	cfgOutFeeDateStart = "2026-09-24"
	cfgOutFeeDateEnd   = "2026-09-24"
	// 代码里拼接的时刻（见 FeeUsecase.GetFeeRate）
	suffixStart = " 00:00:00"
	suffixEnd   = " 23:59:59"
)

// parseInLoc 必须按**应用时区**解析。若退化成 UTC，窗口会整体偏移 8 小时，
// 而 23:59:59 -> 次日 07:59:59 会覆盖到第二天早上，促销费率多生效半天。
func TestParseInLoc_UsesAppTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("加载时区失败（环境缺 tzdata）: %v", err)
	}

	for _, s := range []string{
		cfgOutFeeDateStart + suffixStart,
		cfgOutFeeDateEnd + suffixEnd,
	} {
		ts := parseInLoc(s, loc)
		if ts == 0 {
			t.Fatalf("parseInLoc(%q) 解析失败", s)
		}
		// 往返：解析后再按同时区格式化，必须与输入逐字相同
		back := time.Unix(ts, 0).In(loc).Format("2006-01-02 15:04:05")
		if back != s {
			t.Fatalf("时区往返不一致: 输入 %q, 得到 %q（很可能没按 Asia/Shanghai 解析）", s, back)
		}
	}
}

// 用**真实配置值**走一遍窗口边界。
func TestOutFeeDateWindow_RealConfigValues(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("加载时区失败: %v", err)
	}
	startTs := parseInLoc(cfgOutFeeDateStart+suffixStart, loc)
	endTs := parseInLoc(cfgOutFeeDateEnd+suffixEnd, loc)

	// 窗口必须是一整天（23:59:59 - 00:00:00 = 86399 秒）
	if got := endTs - startTs; got != 86399 {
		t.Fatalf("窗口长度应为 86399 秒（一整天），实际 %d", got)
	}

	ttl := func(now int64) int64 { return CalcFeeTtl(startTs, endTs, now) }

	// 窗口内、距结束还有很久 -> 上限 60
	if got := ttl(startTs); got != 60 {
		t.Errorf("窗口刚开始: CalcFeeTtl=%d, 期望 60", got)
	}
	// 窗口内、距结束 1 秒 -> 收到 1，保证结束时刻立刻切换
	if got := ttl(endTs - 1); got != 1 {
		t.Errorf("结束前 1 秒: CalcFeeTtl=%d, 期望 1", got)
	}
	// 结束时刻本身已不在窗口内（条件是 now < endTs），退回 60
	// —— 这一点很关键：促销费率的"最后 1 秒"是 endTs-1，不是 endTs
	if got := ttl(endTs); got != 60 {
		t.Errorf("结束时刻: CalcFeeTtl=%d, 期望 60（endTs 本身不算在窗口内）", got)
	}
	// 窗口前 1 秒 -> TTL 收到 1，保证开始时刻立刻生效
	if got := ttl(startTs - 1); got != 1 {
		t.Errorf("开始前 1 秒: CalcFeeTtl=%d, 期望 1", got)
	}
	// 窗口前很久 -> 60
	if got := ttl(startTs - 86400); got != 60 {
		t.Errorf("开始前一天: CalcFeeTtl=%d, 期望 60", got)
	}
	// 窗口后很久 -> 60
	if got := ttl(endTs + 86400); got != 60 {
		t.Errorf("结束后一天: CalcFeeTtl=%d, 期望 60", got)
	}
}

// 配置里的日期是 'Y-m-d'，代码拼上时刻。拼错（比如漏了前导空格）会解析失败，
// 而 parseInLoc 失败返回 0 —— 0 会让 `startTs <= now` 恒真、`now < endTs` 恒假，
// 静默退化成"永远不在窗口内"。这里把"解析失败"和"合法零点"区分开。
func TestParseInLoc_FailureIsZeroNotMidnight(t *testing.T) {
	loc := time.UTC
	if got := parseInLoc("2026-09-24", loc); got != 0 {
		t.Errorf("缺时刻应解析失败返回 0，实际 %d", got)
	}
	if got := parseInLoc("", loc); got != 0 {
		t.Errorf("空串应返回 0，实际 %d", got)
	}
	if got := parseInLoc("20260924 00:00:00", loc); got != 0 {
		t.Errorf("错误格式应返回 0，实际 %d", got)
	}
	// 合法零点不能是 0
	if got := parseInLoc("1970-01-01 00:00:00", loc); got != 0 {
		// 这个恰好就是 0，用别的日期验证"合法值不为 0"
		_ = got
	}
	if got := parseInLoc("2026-09-24 00:00:00", loc); got == 0 {
		t.Error("合法日期不应解析为 0")
	}
}
