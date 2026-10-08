package biz

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// 本文件对应原项目 app/api/logic/order/FeeAmountLogic.php 的**纯计算部分**：
//   calculateFeeRate($rate, $tradePrice)
//   calcFeeTtl($startTs, $endTs, $now)
//
// 这两个函数不碰 Cache/DB，所以能在 PHP CLI 里直接跑出对照，
// 期望值全部来自真机输出（见 fee_test.go）。
//
// 不实现的部分：showTotalAmount / getFeeRate 依赖 Cache::get（本项目 cache 驱动是
// redis），本机 php CLI 没有 redis 扩展，跑不出对照，留到下批。

// CalculateFeeRate 逐字对应 FeeAmountLogic::calculateFeeRate。
//
// 原实现：
//
//	if (bccomp($tradePrice, '0', 2) === 0) { return '0.00'; }   // ← 注意 scale=2
//	$product    = bcmul($tradePrice, $rate, 10);
//	$multiplied = bcmul($product, '100', 10);
//	$ceiled     = ceil((float)$multiplied);
//	$r          = bcdiv((string)$ceiled, '100', 2);
//	if (bccomp($r, '0.01', 2) < 0) { return '0.01'; }            // 最低 0.01
//	return $r;
//
// ⚠️ 三个极易漏掉的点，都是跑真机才确认的（按代码推导会推错）：
//
//  1. 第一行的 bccomp 用的是 **scale=2**：两边都截断到 2 位小数再比。
//     所以 tradePrice=0.008 会被当成 '0.00' 直接返回 '0.00'，
//     **不会**走后面的向上取整。也就是"价格 < 0.01 一律返回 0.00"。
//     实测：0.0001/0.001/0.002/0.005/0.008 全部 -> '0.00'，0.01 -> '0.01'。
//
//  2. `ceil((float)$multiplied)` 先把十进制字符串转成 **double** 再取整。
//     大额时会有精度损失，这里用 ParseFloat 复刻同样的行为，不"顺手修好"。
//
//  3. 最后一步是最低 0.01 兜底：rate=0 时 $r='0.00' < '0.01' -> 返回 '0.01'。
//     实测 calculateFeeRate('0','100') === '0.01' —— 零费率**仍然收 0.01**。
//
// bcmath 的截断是向零截断（不是四舍五入），下面 truncateRat 保持一致。
func CalculateFeeRate(rate, tradePrice string) string {
	price, ok := new(big.Rat).SetString(strings.TrimSpace(tradePrice))
	if !ok {
		return "0.00"
	}
	// 对应 bccomp($tradePrice, '0', 2) === 0：按 2 位小数截断后为零
	if truncateRat(price, 2) == "0.00" || truncateRat(price, 2) == "0" {
		return "0.00"
	}

	rateRat, ok := new(big.Rat).SetString(strings.TrimSpace(rate))
	if !ok {
		rateRat = new(big.Rat)
	}

	// $product = bcmul($tradePrice, $rate, 10)
	product := truncateRat(new(big.Rat).Mul(price, rateRat), 10)
	// $multiplied = bcmul($product, '100', 10)
	multiplied := truncateRat(new(big.Rat).Mul(mustRat(product), big.NewRat(100, 1)), 10)

	// $ceiled = ceil((float)$multiplied)  —— 刻意走 float，复刻精度损失
	f, err := strconv.ParseFloat(multiplied, 64)
	if err != nil {
		return "0.00"
	}
	ceiled := math.Ceil(f)
	ceiledStr := strconv.FormatFloat(ceiled, 'f', -1, 64)

	// $r = bcdiv((string)$ceiled, '100', 2)
	r := truncateRat(new(big.Rat).Quo(mustRat(ceiledStr), big.NewRat(100, 1)), 2)

	// 最低 0.01
	if compareRat(r, "0.01", 2) < 0 {
		return "0.01"
	}
	return r
}

// CalcFeeTtl 逐字对应 FeeAmountLogic::calcFeeTtl。
//
//	$ttl = 60;
//	if ($startTs <= $now && $now < $endTs) {
//	    $ttl = min($ttl, max(1, $endTs - $now));      // 活动期内
//	} elseif ($now < $startTs) {
//	    $ttl = min($ttl, max(1, $startTs - $now - 1)); // 活动未开始
//	}
//	return $ttl;
//
// 目的：让缓存过期点对齐费率的切换时刻，避免切换后仍用旧费率。
// 活动已结束时两个分支都不进，TTL 就是 60。
func CalcFeeTtl(startTs, endTs, now int64) int64 {
	ttl := int64(60)
	if startTs <= now && now < endTs {
		ttl = min64(ttl, max64(1, endTs-now))
	} else if now < startTs {
		ttl = min64(ttl, max64(1, startTs-now-1))
	}
	return ttl
}

// ShowTotalAmountWithRate 是 FeeAmountLogic::showTotalAmount 的**纯计算部分**，
// 费率由调用方给出（真实调用链里来自 getFeeRate，那部分依赖 Cache，见下）。
//
// 原实现（$feeRate 换成入参 rate）：
//
//	if (empty($appId) || empty($count) || $count <= 0 || empty($unitPrice) || $unitPrice <= 0) {
//	    return '0';                       // ← 注意是字符串 '0'，不是 '0.00'
//	}
//	$totalPrice = bcmul($unitPrice, $count, 10);
//	$totalPrice = ceil(bcmul($totalPrice, '100', 10)) / 100;
//	$totalPrice = number_format($totalPrice, 2, '.', '');
//	$feeAmount  = calculateFeeRate($rate, $totalPrice);
//	return bcsub($totalPrice, $feeAmount, 2);
//
// 三个要点（都是真机实测确认的）：
//  1. 参数不合法时返回的是 **'0'**（没有小数位），合法路径返回的一定是 2 位小数字符串。
//     这直接决定 JSON 里是 "0" 还是 "0.00"，前端比较字符串时会不同。
//  2. 总价是 **向上取整到分**（×100 → ceil → ÷100），不是四舍五入。
//  3. 手续费是在**取整后的总价**上算的，不是原始总价。
//
// 实测：rate=0.0600 count=2 price=1.005 -> '1.88'
//
//	（总价 2.01，手续费 0.13，到账 1.88）
func ShowTotalAmountWithRate(rate string, count int, unitPrice string) string {
	if count <= 0 || !phpPositiveAmount(unitPrice) {
		return "0"
	}
	totalPrice := bcMul(unitPrice, strconv.Itoa(count), 10)

	mult := bcMul(totalPrice, "100", 10)
	f, err := strconv.ParseFloat(mult, 64)
	if err != nil {
		return "0"
	}
	// ceil(...) / 100 之后再 number_format(2)
	total := math.Ceil(f) / 100
	totalPrice = strconv.FormatFloat(total, 'f', 2, 64)

	feeAmount := CalculateFeeRate(rate, totalPrice)
	return bcSub(totalPrice, feeAmount, 2)
}

// phpPositiveAmount 对应 PHP 的 `empty($v) || $v <= 0`。
//
// ⚠️ 这里必须复刻 empty() 对字符串 "0" 的判定：
// empty("0") 是 true，但 empty("0.0") 是 false（后者靠 `<= 0` 拦下）。
// 两种都算不合法，但路径不同 —— 不写成统一的数值比较是为了以后加日志时能区分。
func phpPositiveAmount(s string) bool {
	if s == "" || s == "0" {
		return false
	}
	r, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return false
	}
	return r.Sign() > 0
}

// bcMul 对应 bcmul($a, $b, $scale)：精确相乘后向零截断到 scale 位。
func bcMul(a, b string, scale int) string {
	ra, ok1 := new(big.Rat).SetString(strings.TrimSpace(a))
	rb, ok2 := new(big.Rat).SetString(strings.TrimSpace(b))
	if !ok1 || !ok2 {
		return "0"
	}
	return truncateRat(new(big.Rat).Mul(ra, rb), scale)
}

// bcSub 对应 bcsub($a, $b, $scale)：精确相减后向零截断到 scale 位。
func bcSub(a, b string, scale int) string {
	ra, ok1 := new(big.Rat).SetString(strings.TrimSpace(a))
	rb, ok2 := new(big.Rat).SetString(strings.TrimSpace(b))
	if !ok1 || !ok2 {
		return "0"
	}
	return truncateRat(new(big.Rat).Sub(ra, rb), scale)
}

/* ------------------------------------------------------------ 十进制工具 */
// truncateRat 把有理数**向零截断**到 scale 位小数，返回十进制字符串。
// 对应 bcmath 的截断语义（bcmath 不四舍五入）。
func truncateRat(r *big.Rat, scale int) string {
	if r == nil {
		return "0"
	}
	pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	n := new(big.Int).Mul(r.Num(), pow)
	n.Quo(n, r.Denom()) // Quo 是向零截断

	neg := n.Sign() < 0
	if neg {
		n.Neg(n)
	}
	s := n.String()
	if scale > 0 {
		for len(s) <= scale {
			s = "0" + s
		}
		s = s[:len(s)-scale] + "." + s[len(s)-scale:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

// compareRat 对应 bccomp($a, $b, $scale)：两边先截断到 scale 位再比。
func compareRat(a, b string, scale int) int {
	ra, ok1 := new(big.Rat).SetString(a)
	rb, ok2 := new(big.Rat).SetString(b)
	if !ok1 || !ok2 {
		return 0
	}
	da := truncateRat(ra, scale)
	db := truncateRat(rb, scale)
	return mustRat(da).Cmp(mustRat(db))
}

func mustRat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return new(big.Rat)
	}
	return r
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
