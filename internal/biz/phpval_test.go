package biz

import "testing"

// 这些用例的价值在于：它们是「Go 代码有没有真的复刻 PHP」的唯一可回归证据。
// ConfigService::get 的分支完全建立在这些语义上，一旦有人"顺手简化"，
// 配置读取就会和线上不一致，而且极难排查。所以这里把边界全钉死。

func TestPhpTruthy(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		// PHP 里为假的值
		{"nil", nil, false},
		{"false", false, false},
		{"int 0", 0, false},
		{"int64 0", int64(0), false},
		{"float 0", 0.0, false},
		{"empty string", "", false},
		{"string \"0\"", "0", false}, // 反直觉但关键
		{"empty slice", []any{}, false},
		{"empty map", map[string]any{}, false},

		// PHP 里为真的值
		{"true", true, true},
		{"int 1", 1, true},
		{"negative", -1, true},
		{"non-empty string", "abc", true},
		{"string \"0.0\"", "0.0", true},     // 反直觉：非空且 != "0"
		{"string \"false\"", "false", true}, // 反直觉：字符串 "false" 为真
		{"string \" \"", " ", true},
		{"slice with zero", []any{0}, true},
		{"map with key", map[string]any{"a": 1}, true},
	}
	for _, c := range cases {
		if got := PhpTruthy(c.in); got != c.want {
			t.Errorf("PhpTruthy(%s=%#v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestPhpIsZeroLike(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"int 0", 0, true},
		{"int64 0", int64(0), true},
		{"float 0", 0.0, true},
		{"string \"0\"", "0", true},
		// 严格比较：这些都不等于 0 / '0'
		{"float 0.0 vs int 0 不算", 0.0001, false},
		{"string \"0.0\"", "0.0", false},
		{"empty string", "", false},
		{"nil", nil, false},
		{"int 1", 1, false},
	}
	for _, c := range cases {
		if got := PhpIsZeroLike(c.in); got != c.want {
			t.Errorf("PhpIsZeroLike(%s=%#v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

func TestPhpJSONDecode(t *testing.T) {
	cases := []struct {
		in     string
		want   any
		wantOk bool
	}{
		{`{"start":"09:30","end":"11:30"}`, map[string]any{"start": "09:30", "end": "11:30"}, true},
		{`[{"start":"09:30"}]`, []any{map[string]any{"start": "09:30"}}, true},
		{`123`, int64(123), true},
		{`6.66`, 6.66, true},
		{`"hello"`, "hello", true},
		{`null`, nil, true},
		{`true`, true, true},
		{`abc`, nil, false},              // 非 JSON
		{``, nil, false},                 // 空串：PHP 的 json_decode('') 是错误
		{`{"a":1} trailing`, nil, false}, // 有多余内容
	}
	for _, c := range cases {
		got, ok := PhpJSONDecode(c.in)
		if ok != c.wantOk {
			t.Errorf("PhpJSONDecode(%q) ok = %v, want %v", c.in, ok, c.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if !deepEqual(got, c.want) {
			t.Errorf("PhpJSONDecode(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestDecodeConfigValue(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any
	}{
		{"纯字符串保持原样", "abc", "abc"},
		// 注意 want 里必须写 int64：PhpJSONDecode 走 normalizeJSONNumbers，
		// 整数统一落成 int64（PHP 是 int，但 Go 里经 json 解码后按 int64 处理）
		{"JSON 对象被解码", `{"a":1}`, map[string]any{"a": int64(1)}},
		{"JSON 数组被解码", `[{"start":"09:30","end":"11:30"}]`, []any{map[string]any{"start": "09:30", "end": "11:30"}}},
		{"数字字符串被解码成数字", "123", int64(123)},
		{"非字符串原样返回", int64(5), int64(5)},
		{"nil 原样返回", nil, nil},
	}
	for _, c := range cases {
		got := DecodeConfigValue(c.in)
		if !deepEqual(got, c.want) {
			t.Errorf("%s: DecodeConfigValue(%#v) = %#v, want %#v", c.name, c.in, got, c.want)
		}
	}
}

// deepEqual 只处理测试里用到的基本类型，避免引入 reflect 之外的依赖。
func deepEqual(a, b any) bool {
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int64:
		bv, ok := b.(int64)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k := range av {
			if !deepEqual(av[k], bv[k]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
