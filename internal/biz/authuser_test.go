package biz

import "testing"

// ⚠️⚠️ 本项目有**两个不同的**手机号掩码，长度参数不同、输出形态也不同：
//
//	authMyUserList(AuthMyUserLists): substr_replace($m,'****',3,6)  -> 11 位变 9 位
//	authUserList  (AppUserLists):    substr_replace($m,'****',3,4)  -> 11 位变 11 位
//
// **不能复用同一个函数。** 这个测试专门锁住两者的区别，
// 防止后人"统一"成一个函数、或把 length 抄错。
//
// 期望值全部来自真实 PHP 实测。
func TestMaskMobileLen4(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"1", "1****"},
		{"123", "123****"},
		{"1234567", "123****"},         // n=7 -> substr($s,7) 是空串
		{"12345678", "123****8"},       // n=8 -> 保留第 7 位起的 "8"
		{"1234567890", "123****890"},   // n=10
		{"18300080008", "183****0008"}, // 真实数据（x_app_auth + x_user 里就是它）
		{"13800138000", "138****8000"},
	}
	for _, c := range cases {
		if got := MaskMobileLen4(c.in); got != c.want {
			t.Errorf("MaskMobileLen4(%q) = %q, 真实 PHP 是 %q", c.in, got, c.want)
		}
	}
}

// 两个掩码必须**产出不同结果**（同一个 11 位手机号）。
func TestTwoMaskLengthsDiffer(t *testing.T) {
	in := "18300080008"
	l4 := MaskMobileLen4(in)
	l6 := MaskMobile(in)

	if l4 == l6 {
		t.Fatalf("两个掩码产出了相同结果 %q —— 说明其中一个抄错了 length", l4)
	}
	if l4 != "183****0008" {
		t.Errorf("MaskMobileLen4(%q) = %q, 期望 183****0008", in, l4)
	}
	if l6 != "183****08" {
		t.Errorf("MaskMobile(%q) = %q, 期望 183****08", in, l6)
	}
	// length=4 等长；length=6 短 2 位
	if len(l4) != len(in) {
		t.Errorf("length=4 应等长：%d vs %d", len(l4), len(in))
	}
	if len(l6) != len(in)-2 {
		t.Errorf("length=6 应短 2 位：%d vs %d", len(l6), len(in)-2)
	}
}
