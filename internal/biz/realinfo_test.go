package biz

import "testing"

// HideStr / HideName 对应 xlu\StrUtil，是**跨语言语义**最容易写错的一处：
// PHP 用的是 mb_* 系列（按**字符**），Go 的 len()/s[i] 是**字节**。
// 中文名用字节处理会直接乱码，所以这里用 rune 实现并用单测锁住。
//
// 期望值来自对原文的逐行推导（源码很短，语义无歧义）：
//
//	hideStr:  $length = mb_strlen($str);
//	          if ($length <= 2) { return $str; }
//	          return mb_substr($str,0,1) . str_repeat('*',$length-2) . mb_substr($str,-1,1);
//	hideName: if (empty($name)) return '';
//	          return mb_substr($name,0,1) . str_repeat('*', mb_strlen($name)-1);
func TestHideStr(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},       // 长度 0 <= 2 -> 原样
		{"A", "A"},     // 1 <= 2 -> 原样
		{"AB", "AB"},   // 2 <= 2 -> **原样返回**（不脱敏！）
		{"ABC", "A*C"}, // 3 -> 首 + 1星 + 末
		{"ABCD", "A**D"},
		// 身份证：18 位 ASCII
		{"21050219690919182X", "2****************X"},
		{"510623198905300010", "5****************0"},
		// ⚠️ 多字节：必须按字符算
		{"郎云霞", "郎*霞"},   // 3 个字符（9 字节）
		{"欧阳娜娜", "欧**娜"}, // 4 个字符
		{"张三", "张三"},     // 2 个字符 -> 原样（不脱敏）
		{"李", "李"},       // 1 个字符 -> 原样
	}
	for _, c := range cases {
		if got := HideStr(c.in); got != c.want {
			t.Errorf("HideStr(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

func TestHideName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},         // empty -> ''
		{"李", "李"},       // 1 字符 -> 首字符 + 0 个星
		{"贾芮铭", "贾**"},   // 3 字符 -> 首 + 2 星
		{"欧阳娜娜", "欧***"}, // 4 字符 -> 首 + 3 星
		{"John", "J***"}, // ASCII 同样按字符
		{"A", "A"},
	}
	for _, c := range cases {
		if got := HideName(c.in); got != c.want {
			t.Errorf("HideName(%q) = %q, 期望 %q", c.in, got, c.want)
		}
	}
}

// 反向断言：如果用**字节**实现，中文用例会得到不同结果 —— 这条测试确保
// 实现没有被"优化"成按字节处理。
func TestHideStr_IsRuneBasedNotByteBased(t *testing.T) {
	in := "郎云霞" // 3 字符 / 9 字节
	got := HideStr(in)
	if got == "郎*******霞" { // 9-2=7 个星 —— 按字节算的结果
		t.Fatal("实现按字节算了（应为 1 个星，得到 7 个）—— mb_* 语义丢失")
	}
	if got != "郎*霞" {
		t.Fatalf("HideStr(%q) = %q, 期望 郎*霞", in, got)
	}
}
