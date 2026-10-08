package biz

import (
	"crypto/md5"
	"encoding/hex"
)

// CreatePassword 逐字对应原项目 app/common.php:16 的 create_password()。
//
//	function create_password(string $plaintext, string $salt): string
//	{
//	    return md5($salt . md5($plaintext . $salt));
//	}
//
// ⚠️⚠️ 最容易写错的一处：**内层 md5() 拼的是十六进制字符串，不是原始字节**。
//
// PHP 的 md5($s) 默认 $raw_output = false，返回 **32 位小写十六进制字符串**。
// 所以外层实际是 md5($salt . <32位hex>)，而不是 md5($salt . <16字节>).
// 写成拼原始字节会得到完全不同的结果，而且**不会有任何报错** ——
// 只是所有用户都登录失败。
//
// ⚠️ 盐来自 config('project.unique_identification')，即 .env 的
// UNIQUE_IDENTIFICATION（本项目实测 7 位）。已由 biz.AuthConfig.UniqueIdent 持有，
// 不需要新增配置。
//
// 这不是 PHP 的 password_hash/bcrypt，是项目自定义的 md5 双重拼接，
// 所以 Go 侧可以原生复刻，不需要跨语言调用。
//
// 单测的期望值全部来自 PHP CLI 真机输出（见 password_test.go）。
func CreatePassword(plaintext, salt string) string {
	inner := md5.Sum([]byte(plaintext + salt))
	// 关键：hex 编码后再参与外层拼接
	outer := md5.Sum([]byte(salt + hex.EncodeToString(inner[:])))
	return hex.EncodeToString(outer[:])
}
