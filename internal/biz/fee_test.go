package biz

import "testing"

// 期望值**全部来自真机 PHP 输出**，不是按代码推导的。
//
// 采集方式：PHP CLI 启动 ThinkPHP 后直接调
//
//	FeeAmountLogic::calculateFeeRate($rate, $tradePrice)
//	FeeAmountLogic::calcFeeTtl($startTs, $endTs, $now)
//
// 这两个是纯函数，不碰 Cache/DB，所以 CLI 能跑。
//
// 之所以必须跑真机：按代码推导 (0.0600, 0.001) 应得 0.01，
// 真机却是 0.00 —— 原因是第一行的 bccomp 用了 scale=2。
func TestCalculateFeeRate_MatchesRealPHP(t *testing.T) {
	cases := []struct{ rate, price, want string }{
		{"0.0600", "100", "6.00"},
		{"0.0600", "100.01", "6.01"},
		{"0.0600", "0.01", "0.01"},
		{"0.0600", "0", "0.00"},
		{"0.0000", "100", "0.01"}, // 零费率仍收 0.01（最低兜底）
		{"0.0600", "0.10", "0.01"},
		{"0.0600", "1.005", "0.07"},
		{"0.0600", "99.99", "6.00"},
		{"0.0600", "123456789.12", "7407407.35"},
		// ↓ 这几条是"价格 < 0.01 一律 0.00"的边界，最容易被推导错
		{"0.0600", "0.001", "0.00"},
		{"0.0600", "0.0001", "0.00"},
		{"0.0600", "0.002", "0.00"},
		{"0.0600", "0.005", "0.00"},
		{"0.0600", "0.008", "0.00"},
		{"0.0600", "16.66", "1.00"},
		{"0.0600", "0.16", "0.01"},
		{"0.0600", "0.17", "0.02"},
		{"1.0000", "10", "10.00"},
		{"0.0600", "3.333", "0.20"},
		{"0.0600", "0.166", "0.01"},
		{"0.0600", "0.167", "0.02"},
		{"0.0600", "0.5", "0.03"},
		{"0.0600", "1", "0.06"},
		{"0.0600", "2", "0.12"},
		{"0.0600", "10", "0.60"},
		{"0", "100", "0.01"},
		{"0.0001", "100", "0.01"},
		{"0.001", "100", "0.10"},
		{"0.005", "100", "0.50"},
		{"0.01", "100", "1.00"},
	}
	for _, c := range cases {
		got := CalculateFeeRate(c.rate, c.price)
		if got != c.want {
			t.Errorf("CalculateFeeRate(%q, %q) = %q, PHP 实测 %q", c.rate, c.price, got, c.want)
		}
	}
}

func TestCalcFeeTtl_MatchesRealPHP(t *testing.T) {
	const now = int64(1800000000)
	cases := []struct {
		name             string
		start, end, want int64
	}{
		{"活动期内(未到结束)", now - 100, now + 3600, 60},
		{"活动期内(1秒后结束)", now - 100, now + 1, 1},
		{"活动未开始", now + 3600, now + 7200, 60},
		{"活动未开始(1秒后开始)", now + 1, now + 3600, 1},
		{"活动已结束", now - 7200, now - 3600, 60},
	}
	for _, c := range cases {
		if got := CalcFeeTtl(c.start, c.end, now); got != c.want {
			t.Errorf("%s: CalcFeeTtl = %d, PHP 实测 %d", c.name, got, c.want)
		}
	}
}

// 截断必须是向零截断（bcmath 语义），不是四舍五入。
func TestTruncateRatIsTruncationNotRounding(t *testing.T) {
	cases := []struct {
		in    string
		scale int
		want  string
	}{
		{"0.999", 2, "0.99"},
		{"0.005", 2, "0.00"},
		{"1.239", 2, "1.23"},
		{"100", 2, "100.00"},
		{"0.1", 4, "0.1000"},
	}
	for _, c := range cases {
		if got := truncateRat(mustRat(c.in), c.scale); got != c.want {
			t.Errorf("truncateRat(%q, %d) = %q, want %q", c.in, c.scale, got, c.want)
		}
	}
}

// 期望值来自真机 PHP：在 CLI 里复刻 showTotalAmount 的函数体、
// 把 getFeeRate 的结果换成固定入参（这样全程是纯 bcmath，不需要 Cache/DB）。
func TestShowTotalAmountWithRate_MatchesRealPHP(t *testing.T) {
	cases := []struct {
		rate      string
		count     int
		unitPrice string
		want      string
	}{
		{"0.0600", 1, "100", "94.00"},
		{"0.0600", 3, "16.66", "46.98"},
		{"0.0600", 1, "0.01", "0.00"},
		// ↓ 参数不合法时返回的是 '0'（没有小数位），不是 '0.00'
		{"0.0600", 0, "100", "0"},
		{"0.0600", 1, "0", "0"},
		{"0.0600", 1, "", "0"},
		{"0.0000", 1, "100", "99.99"},
		{"0.0600", 2, "1.005", "1.88"},
		{"0.0600", 7, "3.333", "21.93"},
		{"0.0600", 100, "123456789.12", "11604938177.28"},
		{"0.0100", 5, "99.99", "494.95"},
	}
	for _, c := range cases {
		got := ShowTotalAmountWithRate(c.rate, c.count, c.unitPrice)
		if got != c.want {
			t.Errorf("ShowTotalAmountWithRate(%q, %d, %q) = %q, PHP 实测 %q",
				c.rate, c.count, c.unitPrice, got, c.want)
		}
	}
}

// 总价是**向上取整到分**（×100 → ceil → ÷100），不是四舍五入。
// 用 rate=0（手续费固定 0.01）反推总价：2.001 若四舍五入到 2.00 则到账 1.99，
// 向上取整到 2.01 则到账 2.00。
func TestShowTotalAmount_CeilsToCentNotRounds(t *testing.T) {
	if got := ShowTotalAmountWithRate("0.0000", 1, "2.001"); got != "2.00" {
		t.Fatalf("2.001 应向上取整到 2.01（到账 2.00），实际 %q", got)
	}
}
