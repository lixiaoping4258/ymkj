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
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"

	"github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
		return marshalProto(m)
	}
	// 切片：原项目有不少接口的 data 就是**顶层数组**（例如支付方式列表），
	// 而 Kratos 的 reply 是 message，天然会被包成 {"data":{...}}。
	// 所以这里对 []*SomeReply 这类切片逐个用 protojson 序列化再拼成 JSON 数组，
	// 保证 data 的 JSON 形态仍是数组。
	if arr, err, handled := marshalProtoSlice(v); handled {
		return arr, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func marshalProto(m proto.Message) (json.RawMessage, error) {
	b, err := protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   false, // json_name 已在 .proto 里逐个显式指定
	}.Marshal(m)
	if err != nil {
		return nil, err
	}
	return unquoteInt64(m, b)
}

// is64BitInt 判断 proto 标量类型在 JSON 里会被序列化成字符串。
func is64BitInt(k protoreflect.Kind) bool {
	switch k {
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return true
	default:
		return false
	}
}

func isMessageKind(k protoreflect.Kind) bool {
	return k == protoreflect.MessageKind || k == protoreflect.GroupKind
}

// unquoteInt64 把 proto3 JSON 里的 64 位整数字符串还原成 JSON 数字。
//
// 为什么必须做：proto3 的 JSON 映射**规定** int64/uint64/sint64/fixed64 等
// 序列化成字符串（为了避开 JavaScript 2^53 的精度问题）。但原项目
// `json_encode` 对 PHP int 输出的是**数字**：
//
//	{"id":61089517,"sn":178056289917980,"create_time":1783678044}
//
// 前端只要做了 `===` 比较或算术运算，字符串就会坏掉。
// 已用真实 PHP（复刻 ThinkPHP 的 PDO 参数：STRINGIFY_FETCHES=false、
// EMULATE_PREPARES=false）实测确认过这一点。
//
// 这里按 **protoreflect 的字段类型**精确匹配，而不是对输出做字符串替换 ——
// 后者会把同样长得像数字的普通字符串字段（比如 user_money 的 "0.00"）一起误伤。
func unquoteInt64(m proto.Message, raw []byte) (json.RawMessage, error) {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	fixInt64Fields(m.ProtoReflect(), v)
	out, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(out), nil
}

// fixInt64Fields 递归地把 64 位整数字段从字符串改成 json.Number。
func fixInt64Fields(md protoreflect.Message, v any) {
	obj, ok := v.(map[string]any)
	if !ok {
		// 不是对象（例如 google.protobuf.Value 装的是标量），无需处理
		return
	}
	fields := md.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		key := fd.JSONName()
		val, present := obj[key]
		if !present {
			continue
		}
		switch {
		case is64BitInt(fd.Kind()):
			if s, ok := val.(string); ok {
				obj[key] = json.Number(s)
			}
		case fd.IsMap():
			// 本项目暂无 map<_, message> 字段；真要用到再补
		case fd.IsList() && isMessageKind(fd.Kind()):
			arr, ok := val.([]any)
			if !ok {
				continue
			}
			list := md.Get(fd).List()
			for j := 0; j < len(arr) && j < list.Len(); j++ {
				fixInt64Fields(list.Get(j).Message(), arr[j])
			}
		case isMessageKind(fd.Kind()):
			// 未设置的 message 字段在这里是 null，walk 进去也拿不到 map，是安全的
			fixInt64Fields(md.Get(fd).Message(), val)
		}
	}
}

// marshalProtoSlice 处理「元素是 proto.Message 的切片」。
// 第二个返回值是错误，第三个表示是否已处理。
func marshalProtoSlice(v any) (json.RawMessage, error, bool) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil, nil, false
	}
	n := rv.Len()
	parts := make([]json.RawMessage, 0, n)
	for i := 0; i < n; i++ {
		el := rv.Index(i).Interface()
		m, ok := el.(proto.Message)
		if !ok {
			// 只要有一个元素不是 proto 消息，就交给通用 json 处理，
			// 避免出现半 proto 半原生 的怪异结果
			return nil, nil, false
		}
		b, err := marshalProto(m)
		if err != nil {
			return nil, err, true
		}
		parts = append(parts, b)
	}
	out, err := json.Marshal(parts)
	if err != nil {
		return nil, err, true
	}
	return out, nil, true
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
