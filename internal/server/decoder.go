package server

import (
	"encoding/json"
	"io"
	nethttp "net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/transport/http"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RequestDecoder 替代 Kratos 的默认请求解码器，让 POST 接口同时接受
// **application/json / multipart/form-data / application/x-www-form-urlencoded**。
//
// 🔴 为什么必须自己写：
//
// Kratos 的默认解码器只注册了少数几种 Content-Type，遇到 `multipart/form-data`
// 会直接返回：
//
//	unregister Content-Type: multipart/form-data; boundary=----xxx
//
// 而**原 PHP 项目是全都接受的** —— ThinkPHP 的 `$request->post()` 对 JSON、
// form-data、x-www-form-urlencoded 一视同仁（前端用 axios 传 FormData、
// 或者用 Apifox/Postman 调试时默认就是 form-data）。
//
// 前端只要用 FormData 提交，登录就 100% 失败 —— 这是**接口不可用**级别的兼容问题，
// 而且错误信息（"unregister Content-Type"）完全不像业务错误，排查会很费劲。
//
// ⚠️ 另一个必须对齐的点：**PHP 会忽略多余参数**（`$request->post()` 把整个请求体
// 都给你，用不到的就放着）。protojson 默认遇到未知字段会**报错**，两者行为相反。
// 所以这里统一用 `DiscardUnknown: true`，与 PHP 的宽容行为一致。
func RequestDecoder(req *nethttp.Request, v any) error {
	ct := req.Header.Get("Content-Type")

	// 只对 proto 消息做特殊处理；其它类型交给默认解码器（保持一致）
	msg, isProto := v.(proto.Message)
	if !isProto {
		return http.DefaultRequestDecoder(req, v)
	}

	switch {
	case strings.HasPrefix(ct, "multipart/form-data"),
		strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		return decodeForm(req, msg)

	case strings.HasPrefix(ct, "application/json"),
		ct == "":
		return decodeJSON(req, msg)

	default:
		// 其它类型（如 application/x-protobuf）沿用 Kratos 默认行为
		return http.DefaultRequestDecoder(req, v)
	}
}

// decodeForm 把表单字段转成 JSON 再交给 protojson。
//
// 为什么绕一圈 JSON：表单值全是字符串，而 proto 字段有 int32/uint64/bool 等类型。
// protojson **本来就接受字符串形式的整数**（如 `"scene":"2"` 能解析进 int32），
// 所以借道 protojson 比自己写类型映射更可靠、也更少重复实现。
func decodeForm(req *nethttp.Request, msg proto.Message) error {
	// ParseMultipartForm 内部会先调 ParseForm；
	// 对 x-www-form-urlencoded 走 ParseForm 分支即可。
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		if err2 := req.ParseForm(); err2 != nil {
			return err
		}
	}

	m := make(map[string]any, len(req.Form))
	for k, vs := range req.Form {
		if len(vs) == 0 {
			continue
		}
		// 与 ThinkPHP 一致：同名多值取第一个（PHP 里后值覆盖前值，
		// 但表单场景基本是单值；这里取第一个更可预测，且已在 README 记录）
		m[k] = vs[0]
	}
	// 文件字段（multipart 的文件部分）不参与 JSON 化 —— 本接口没有文件参数。
	// 若将来有上传接口，需要单独处理 req.MultipartForm.File。

	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, msg)
}

// decodeJSON 与默认行为一致，但容忍未知字段（对齐 PHP）。
func decodeJSON(req *nethttp.Request, msg proto.Message) error {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}
	// 空 body：Kratos 原本会报错，但 PHP 下空 body 等于"所有参数缺失"，
	// 由业务校验去报「请输入账号」等具体信息，比解码器报错更贴近原行为。
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(body, msg)
}
