package biz

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

// 期望值**全部来自 PHP CLI 真机输出**，不是按代码推导的。
//
// 采集方式：PHP CLI 启动 ThinkPHP 后直接调 create_password($p, env('project.unique_identification')).
// 该函数是纯 md5，不碰 DB/Cache，所以 CLI 能跑。
//
// 本机盐 = "aaaa123"（7 位，与 README 记录的 UNIQUE_IDENTIFICATION 长度一致）。
const testPwdSalt = "aaaa123"

func TestCreatePassword_MatchesRealPHP(t *testing.T) {
	cases := []struct{ plain, want string }{
		{"test123", "33c8e470ceb408587ca87c797844ae8b"},
		{"Aa123456", "ebea38b1c2f327398b8600f6b2f49aa4"},
		{"123456", "844cb0037d204040aec259dce353b4e1"},
		{"", "22581a17439d86d1e1298349f97b0894"},
		{"P@ssw0rd!", "e211251f81d13a1fad34e77375c713fc"},
		{"中文密码", "9121c710138dc5705e55d066ddcc6537"},
	}
	for _, c := range cases {
		got := CreatePassword(c.plain, testPwdSalt)
		if got != c.want {
			t.Errorf("CreatePassword(%q, salt) = %s, PHP 实测 %s", c.plain, got, c.want)
		}
	}
}

// 单独钉住那个最容易写错的点：外层拼的是**内层的十六进制字符串**，不是原始字节。
//
// 如果实现写成 `md5.Sum([]byte(salt + string(inner[:])))`，结果会不同且不报错，
// 后果是**所有用户都登录失败**。这条测试专门排除那种写法。
func TestCreatePassword_OuterUsesHexNotRawBytes(t *testing.T) {
	plain := "test123"
	want := "33c8e470ceb408587ca87c797844ae8b"

	// 正确：外层拼 hex
	if got := CreatePassword(plain, testPwdSalt); got != want {
		t.Fatalf("正确实现应得 %s，实得 %s", want, got)
	}

	// 错误写法（拼原始字节）—— 必须得到不同的值，否则说明这条测试没有区分力
	wrong := createPasswordRawBytesHook(plain, testPwdSalt)
	if wrong == want {
		t.Fatal("拼原始字节竟然得到相同结果 —— 这条测试失去区分力，请检查")
	}
}

// createPasswordRawBytesHook 是"错误写法"的参照实现，只用于上面的区分力断言。
// 生产代码里**不存在**这个函数，加在这里是为了证明两种写法确实不同。
func createPasswordRawBytesHook(plaintext, salt string) string {
	inner := md5.Sum([]byte(plaintext + salt))
	outer := md5.Sum([]byte(salt + string(inner[:])))
	return hex.EncodeToString(outer[:])
}
