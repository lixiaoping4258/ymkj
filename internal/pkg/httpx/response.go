// Package httpx 负责让 Go 版的响应格式与原 ThinkPHP 项目 **逐字节兼容**。
//
// 原项目的信封由 app/common/service/JsonService.php 定义：
//
//	{"code":1,"show":0,"msg":"","data":{...}}      成功 (data())
//	{"code":1,"show":1,"msg":"success","data":[]}  成功 (success())
//	{"code":0,"show":1,"msg":"fail","data":[]}     失败 (fail())
//
// 两个必须注意的差异：
//  1. **code=1 才是成功**，Kratos 默认是 code=0 成功 —— 直接用默认 encoder 前端全挂。
//  2. **HTTP 状态码恒为 200**（原项目 json($result, 200)），业务码只在 body 里。
//
// 所以这里替换掉 Kratos 默认的 ResponseEncoder / ErrorEncoder。
package httpx

import (
	"encoding/json"
	"net/http"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// 业务码。注意与 Kratos/HTTP 的语义相反。
const (
	CodeSuccess = 1
	CodeFail    = 0
)

// CompatHTTPStatus 为 true 时所有响应都返回 HTTP 200，与原项目一致。
//
// ⚠️ 代价：网关、监控、告警都看不到 HTTP 层的错误，所有失败都"看起来是 200"。
// 等前端切到新契约后，把它设为 false 就能恢复正常的 HTTP 语义。
var CompatHTTPStatus = true

// Envelope 是原项目的响应信封。
type Envelope struct {
	Code int             `json:"code"`
	Show int             `json:"show"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// Enveloper 让 handler 自定义信封字段。
// 需要返回 success(msg, data) / fail(msg) 语义时实现它，否则默认按 data() 处理。
type Enveloper interface {
	Envelope() (code int, show int, msg string, data any)
}

// marshalData 序列化 data 字段。
//
// proto 消息用 protojson 且 **EmitUnpopulated=true**：PHP 的 json_encode 会把
// 0 / "" / [] / null 全都输出，而 protojson 默认会省略零值 —— 不打开这个开关，
// 前端拿到的字段会凭空少掉，属于最难查的一类问题。
func marshalData(v any) (json.RawMessage, error) {
	if v == nil {
		// 原项目 data() 的 $data 默认值是空数组
		return json.RawMessage("[]"), nil
	}
	if m, ok := v.(proto.Message); ok {
		b, err := protojson.MarshalOptions{
			EmitUnpopulated: true,
			UseProtoNames:   false, // 输出 lowerCamelCase，和 PHP 侧一致
		}.Marshal(m)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func writeEnvelope(w http.ResponseWriter, code, show int, msg string, data any) error {
	raw, err := marshalData(data)
	if err != nil {
		// 序列化都失败了，只能退化成最小可用的错误信封
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if CompatHTTPStatus {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = w.Write([]byte(`{"code":0,"show":1,"msg":"response marshal failed","data":[]}`))
		return err
	}
	out, err := json.Marshal(Envelope{Code: code, Show: show, Msg: msg, Data: raw})
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, err = w.Write(out)
	return err
}

// ResponseEncoder 替换 Kratos 的 http.DefaultResponseEncoder。
//
// 对应原项目 JsonService::data()：code=1, show=0, msg=""。
func ResponseEncoder(w http.ResponseWriter, r *http.Request, v any) error {
	code, show, msg, data := CodeSuccess, 0, "", v
	if e, ok := v.(Enveloper); ok {
		code, show, msg, data = e.Envelope()
	}
	return writeEnvelope(w, code, show, msg, data)
}

// ErrorEncoder 替换 Kratos 的 http.DefaultErrorEncoder。
//
// 对应原项目 JsonService::fail()：code=0, show=1, msg=<错误信息>。
// show=1 表示"这个提示要弹给用户看"，所以业务错误的 msg 会被直接展示 ——
// 不要把内部错误细节往这里塞。
func ErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	// 支持 fail($msg, $data) 这种要带 data 的场景：
	// biz 层返回 *bizError 时它能自定义整个信封。
	if e, ok := err.(Enveloper); ok {
		code, show, msg, data := e.Envelope()
		if CompatHTTPStatus {
			w.WriteHeader(http.StatusOK)
		}
		_ = writeEnvelope(w, code, show, msg, data)
		return
	}
	se := errors.FromError(err)
	if CompatHTTPStatus {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(int(se.Code))
	}
	_ = writeEnvelope(w, CodeFail, 1, se.Message, nil)
}

// Fail 构造一个业务错误，最终会以 code=0 / show=1 返回。
//
// 用法：return nil, httpx.Fail("余额不足")
func Fail(msg string) error {
	return errors.New(http.StatusOK, "BIZ_FAIL", msg)
}

// FailCode 自定义业务码与 show。
//
// 登录中间件需要它：原项目用的是
//
//	JsonService::fail('请求认证信息有误，请重新登录', [], -403, 0)
//
// 注意 code 是 **-403** 而不是 0，且 show=0（前端不弹这个提示，
// 而是自己跳登录页）。body 里 code 必须是 -403，前端就是按它判断的。
func FailCode(code, show int, msg string) error {
	return &codedError{code: code, show: show, msg: msg}
}

type codedError struct {
	code int
	show int
	msg  string
}

func (e *codedError) Error() string { return e.msg }

func (e *codedError) Envelope() (int, int, string, any) {
	return e.code, e.show, e.msg, nil
}

// FailWithData 业务失败但需要带 data（原项目 fail($msg, $data) 的用法）。
//
// ErrorEncoder 会识别 Enveloper，所以它最终输出
// {"code":0,"show":1,"msg":...,"data":...}。
func FailWithData(msg string, data any) error {
	return &bizError{msg: msg, data: data}
}

type bizError struct {
	msg  string
	data any
}

func (e *bizError) Error() string { return e.msg }

// Envelope 实现 Enveloper，让 ErrorEncoder 能输出带 data 的失败信封。
func (e *bizError) Envelope() (int, int, string, any) {
	return CodeFail, 1, e.msg, e.data
}
