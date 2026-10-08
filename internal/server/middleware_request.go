package server

import (
	"context"

	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
)

// RequestQueryMiddleware 把原始 query 参数存进 context，供列表类接口使用。
//
// ⚠️ 必须挂在**最外层**（http.go 里排在 recovery/logging 之前）。
// 原因见 internal/pkg/httpx/requestctx.go：一旦被其它中间件用 context.WithValue
// 包装过，ctx 就不再实现 http.Context，这里也取不到 Request 了。
//
// 取不到时不做任何事（放行）—— 上层拿到的是空 query，退化成默认分页，
// 不会让请求失败。但这种降级是静默的，所以顺序错了很难发现，
// 这也是为什么这里要留这么长的注释。
func RequestQueryMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			if h, ok := ctx.(khttp.Context); ok {
				if r := h.Request(); r != nil {
					ctx = httpx.WithQuery(ctx, r.URL.Query())
				}
			}
			return handler(ctx, req)
		}
	}
}
