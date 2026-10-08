package httpx

import (
	"context"
	"net/url"
)

// 为什么需要这个文件：
//
// 列表类接口要读原始 query 参数（分页/筛选/排序十几个键），
// 一开始的做法是在 service 里 `ctx.(khttp.Context)` 拿 Request()。**这是错的。**
//
// Kratos 的中间件（logging/tracing 等）会用 context.WithValue 包装 ctx，
// 包装后的动态类型是 *context.valueCtx，**不再是那个实现了 http.Context 的 wrapper**，
// 所以类型断言会静默失败 —— 表现为所有分页/筛选参数被忽略，退回默认值。
//
// 这个 bug 曾经真实存在：缓存键算出来是空字符串的 md5（d41d8cd9...），
// 才发现 url.Values 一直是空的。
//
// 正确做法：在最外层中间件（ctx 还是 wrapper 的时候）把 query 取出来，
// 用 context.WithValue 存进去。之后无论被包装多少层，Value() 都能穿透取到。
type queryKey struct{}

// WithQuery 把原始 query 参数放进 context。
func WithQuery(ctx context.Context, q url.Values) context.Context {
	return context.WithValue(ctx, queryKey{}, q)
}

// QueryFrom 取回原始 query 参数。
//
// 永远返回非 nil（取不到时返回空 Values），这样调用方可以直接 .Get()，
// 不用到处判空 —— 判空漏一处就是又一次静默忽略参数。
func QueryFrom(ctx context.Context) url.Values {
	if q, ok := ctx.Value(queryKey{}).(url.Values); ok && q != nil {
		return q
	}
	return url.Values{}
}
