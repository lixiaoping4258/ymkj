package biz

import (
	"bytes"
	"encoding/json"
)

// MarshalNoHTMLEscape 与 PHP 的 json_encode 行为对齐。
//
// ⚠️⚠️ Go 的 `json.Marshal` 会把 `<` `>` `&` 转义成 `\u003c` `\u003e` `\u0026`，
// 而 **PHP 的 `json_encode` 不转义这三个字符**（它只默认转义 `/` 为 `\/`）。
//
// 两者对 JSON 解析器是等价的，但：
//  1. **原始响应文本不同** —— 客户端若对 body 做字符串匹配/正则就会失败
//  2. 抓包对比 PHP 与 Go 的响应时会看到差异，容易被误判为"迁移出错"
//
// 所以凡是与 PHP 契约对齐的输出，都要用这个函数而不是 json.Marshal。
//
// 实测案例：/v1/user/notice/getNew 的 content 里是 HTML，
// 用 json.Marshal 会输出 `\u003cp\u003e`，PHP 输出 `<p>`。
func MarshalNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode 会追加一个换行，去掉它
	b := buf.Bytes()
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	return b, nil
}
