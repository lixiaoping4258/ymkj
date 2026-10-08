package biz

import (
	"encoding/json"
	"testing"
)

// 这个文件钉住一个**承载性的不变量**，而不是某个具体函数的行为。
//
// 背景：白名单权限缓存（whitelist.go:89）和库存汇总缓存（stock.go:69）
// 存的都是 map[string]any，读回时各自用 PhpTruthy / toInt64 还原。
// 这一步之所以安全，是因为 **JSON 往返后真值不变**：
//
//	true/false   -> bool（保持）
//	整数          -> float64（类型变了，真值没变）
//	字符串        -> string（保持）
//	null          -> nil（保持）
//
// 但这个不变量**现在只是碰巧成立** —— 没有任何测试守着它。
// 谁要是哪天把 PhpTruthy 改成 "只认 bool"，白名单缓存就会静默失效：
// 缓存里读出来的值全被当成 false，于是**所有权限都变成"不拦截"**。
// 那是安全相关的静默降级，而且只在缓存命中时出现（第一次请求还是对的）——
// 是最难复现的一类 bug。
//
// 所以这里把"往返前后真值一致"直接断言出来。

func jsonRoundTrip(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	return out
}

// 白名单缓存的真实形态就是 map[string]bool。
func TestPhpTruthy_BoolMapSurvivesJSONRoundTrip(t *testing.T) {
	orig := map[string]bool{
		"no_collection_trade": true,
		"no_sale":             false,
		"no_buy":              true,
	}
	rt, ok := jsonRoundTrip(t, orig).(map[string]any)
	if !ok {
		t.Fatalf("往返后应为 map[string]any，实际 %T", jsonRoundTrip(t, orig))
	}
	for k, want := range orig {
		if got := PhpTruthy(rt[k]); got != want {
			t.Errorf("%s: 往返后 PhpTruthy = %v，原值 %v（类型 %T）—— 缓存会静默失效",
				k, got, want, rt[k])
		}
	}
}

// 其它缓存里可能出现混合类型（如 trade 配置、库存汇总）。
// 逐个类型确认往返后真值不变。
func TestPhpTruthy_ScalarTypesSurviveJSONRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"true", true, true},
		{"false", false, false},
		{"正整数", 5, true},
		{"零", 0, false},
		{"字符串0", "0", false}, // PHP: empty("0") 为真
		{"非空字符串", "abc", true},
		{"空字符串", "", false},
		{"空数组", []any{}, false},
		{"非空数组", []any{1}, true},
		{"null", nil, false},
	}
	for _, c := range cases {
		rt := jsonRoundTrip(t, c.in)
		if got := PhpTruthy(rt); got != c.want {
			t.Errorf("%s: 原值 %#v(%T) 往返后 %#v(%T) -> PhpTruthy=%v, 期望 %v",
				c.name, c.in, c.in, rt, rt, got, c.want)
		}
	}
}

// 整数往返后是 float64 —— 这条单独钉住，因为它是"类型变了但真值没变"的典型，
// 也是最容易被写成 `v.(int)` 而静默失败的地方。
func TestJSONRoundTrip_IntegersBecomeFloat64(t *testing.T) {
	rt := jsonRoundTrip(t, map[string]any{"num": 42})
	m, ok := rt.(map[string]any)
	if !ok {
		t.Fatalf("应为 map[string]any，实际 %T", rt)
	}
	if _, isFloat := m["num"].(float64); !isFloat {
		t.Fatalf("整数往返后应为 float64，实际 %T（若这里变了，toInt64/feeStr 的实现前提就不成立了）", m["num"])
	}
	// toInt64 必须能还原
	if got := toInt64(m["num"]); got != 42 {
		t.Fatalf("toInt64 未能还原: %d", got)
	}
	// feeStr 也必须能还原（它被用来读配置里的数字）
	if got := feeStr(m["num"]); got != "42" {
		t.Fatalf("feeStr 未能还原: %q", got)
	}
}
