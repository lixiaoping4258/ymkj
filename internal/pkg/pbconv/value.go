// Package pbconv 处理 Go 原生值 <-> protobuf 值的转换。
package pbconv

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

// ToValue 把任意 Go 值转成 google.protobuf.Value。
//
// 为什么需要它：原项目 app\common\service\ConfigService::get 的返回类型是不确定的
// （可能是 int、string、数组、对象，取决于 la_config 表里存的 JSON），
// proto 里没法定具体类型，只能落到 Value 这个「任意 JSON」容器上。
//
// structpb.NewValue 支持 nil/bool/整数/浮点/string/[]byte/[]any/map[string]any，
// 其它类型（比如 time.Time）会报错，此时退化成字符串而不是让接口 500。
func ToValue(v any) *structpb.Value {
	if v == nil {
		return structpb.NewNullValue()
	}
	pv, err := structpb.NewValue(v)
	if err != nil {
		return structpb.NewStringValue(fmt.Sprintf("%v", v))
	}
	return pv
}
