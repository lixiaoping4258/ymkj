package v1

import "encoding/json"

// 本文件让部分 reply 以「顶层数组」或「预先组装的 JSON」作为信封的 data 输出。
//
// 背景：原 ThinkPHP 项目里不少接口直接 `return $this->data([...])`，
// 数组会成为信封的 data，也就是：
//
//	{"code":1,"show":0,"msg":"","data":[...]}
//
// 而 Kratos 的 handler 必须返回 message，默认会被包成
// `{"data":{"items":[...]}}` —— 前端拿到对象而不是数组就会解析失败。
//
// 为什么能这么写：Go 的接口是**结构化**的。只要下面这些方法存在，
// `*GetPayWayReply` 就自动满足 `httpx.Enveloper`，不需要 import httpx，
// 也就不会有循环依赖。
//
// 约定（改这里之前先确认原 PHP 的返回形态）：
//   - 原接口 data 是数组  -> 实现 Envelope() 并把切片摊出去
//   - 原接口 data 是对象  -> 不要实现，走默认的 message 包裹
//   - 原接口 data 是空数组 -> 返回 []any{}（空对象 {} 与空数组 [] 前端处理不同）

// Envelope 让支付方式列表以顶层数组返回。
func (r *GetPayWayReply) Envelope() (int, int, string, any) {
	return 1, 0, "", r.GetItems()
}

// Envelope 让秒转分类列表以顶层数组返回。
func (r *GetSaleCategoriesReply) Envelope() (int, int, string, any) {
	return 1, 0, "", r.GetItems()
}

// Envelope 让兑换校验以空数组返回（原实现是 `return $this->data([])`）。
func (r *CheckExchangeReply) Envelope() (int, int, string, any) {
	return 1, 0, "", []any{}
}

// Envelope 让 RawData 直接输出预先组装好的 JSON 作为 data。
//
// 返回 json.RawMessage 时，httpx 的 encoder 会用 json.Marshal 原样输出它
// （RawMessage 实现了 json.Marshaler），不会二次转义。
func (r *RawData) Envelope() (int, int, string, any) {
	b := r.GetJson()
	if len(b) == 0 {
		return 1, 0, "", []any{}
	}
	return 1, 0, "", json.RawMessage(b)
}
