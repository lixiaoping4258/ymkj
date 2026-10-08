package biz

import (
	"encoding/json"
	"strconv"
	"strings"
)

// 本文件复刻 PHP 的若干「隐式类型语义」。
//
// 这些语义看着琐碎，但原项目 app/common/service/ConfigService.php 的
// 分支完全建立在它们之上。用 Go 的直觉去写（比如把 "" 当有效值）
// 会让配置读取行为和线上不一致，而且极难排查。

// PhpTruthy 等价于 PHP 的 if ($value) 判断。
//
// PHP 里为假的值：false / 0 / 0.0 / "" / "0" / [] / null
// 特别注意两个反直觉点：
//   - "0"  是假
//   - "false"、"0.0"、[0] 都是真
func PhpTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case int:
		return t != 0
	case int32:
		return t != 0
	case int64:
		return t != 0
	case uint:
		return t != 0
	case uint64:
		return t != 0
	case float32:
		return t != 0
	case float64:
		return t != 0
	case string:
		return t != "" && t != "0"
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		// 其它类型（结构体、指针等）在 PHP 里都算对象，为真
		return true
	}
}

// PhpIsZeroLike 等价于 PHP 的 ($value === 0 || $value === '0')。
//
// 这是严格比较：float64(0) 不等于 int(0)，但在 JSON decode 之后
// 数字统一是 float64，所以这里两种都认。
func PhpIsZeroLike(v any) bool {
	switch t := v.(type) {
	case int:
		return t == 0
	case int64:
		return t == 0
	case float64:
		return t == 0
	case string:
		return t == "0"
	default:
		return false
	}
}

// PhpJSONDecode 等价于 PHP 的 json_decode($s, true)。
//
// 返回 (值, 是否是合法 JSON)。注意 PHP 里 json_decode('null') 是合法的，
// 结果是 null；json_decode('123') 结果是 int(123)。Go 的 encoding/json
// 行为一致，但要把「非字符串」这条入口排除掉。
func PhpJSONDecode(s string) (any, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		// PHP: json_decode('') === null 且 json_last_error() != 0
		return nil, false
	}
	var out any
	dec := json.NewDecoder(strings.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, false
	}
	// 确保没有多余内容（PHP 也不允许 '1 2'）
	if dec.More() {
		return nil, false
	}
	return normalizeJSONNumbers(out), true
}

// normalizeJSONNumbers 把 json.Number 还原成 float64 / int64。
//
// PHP 的 json_decode 把没有小数点的数字解成 int，有小数点的解成 float。
// protojson 输出时两者都是数字，但 Google\Protobuf\Value 需要区分。
func normalizeJSONNumbers(v any) any {
	switch t := v.(type) {
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	case []any:
		for i := range t {
			t[i] = normalizeJSONNumbers(t[i])
		}
		return t
	case map[string]any:
		for k := range t {
			t[k] = normalizeJSONNumbers(t[k])
		}
		return t
	default:
		return v
	}
}

// DecodeConfigValue 复刻 ConfigService::get 中间那段：
//
//	if (is_string($value)) { 尝试 json_decode；成功就用解码结果，失败保留原字符串 }
//
// 字符串 "123" 会被解成数字 123，字符串 "abc" 保持 "abc"。
func DecodeConfigValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if decoded, ok := PhpJSONDecode(s); ok {
		return decoded
	}
	return s
}

// PhpLooseEqualsInt 复刻 PHP 的宽松比较 `$value == $n`（n 是整数）。
//
// 用在 TradeConfigService 的开关判断上：
//
//	if ($tradeSwitchCheck['tradeSwitch'] == 1 && $tradeSwitchCheck['isOpen'] != 1)
//
// tradeSwitch 来自 ConfigService::get，类型不确定（可能 int 1、字符串 "1"、
// 或 json_decode 出来的 float64 1）。PHP 的 == 对它们都判真，
// Go 里用 == 就只认 int，会把「开关已打开」误判成关闭。
//
// 注意这里只覆盖整数比较这一种用法，不是完整的 PHP 比较表
// （PHP 8 改过字符串与数字比较的规则，完整复刻是个无底洞且没必要）。
func PhpLooseEqualsInt(v any, want int64) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		if t {
			return want == 1
		}
		return want == 0
	case int:
		return int64(t) == want
	case int32:
		return int64(t) == want
	case int64:
		return t == want
	case uint64:
		return int64(t) == want
	case float32:
		return float64(t) == float64(want)
	case float64:
		return t == float64(want)
	case string:
		// 数字字符串按数字比较（"1" == 1 为真）
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return false
		}
		return f == float64(want)
	default:
		return false
	}
}
