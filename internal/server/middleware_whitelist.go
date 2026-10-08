package server

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
)

// 本文件对应原项目 app/api/http/middleware/WhitelistMiddleware.php。
//
// 原项目把限制项绑在路由上：
//
//	Route::get('purchase', [PurchaseController::class, 'index'])
//	    ->middleware(WhitelistMiddleware::class)
//	    ->append(['whitelist_item' => 'no_collection_trade']);
//
// Kratos 没有"路由 append"这种东西，所以改成一张 operation -> 限制项 的表。
// 注意 append 支持单个（whitelist_item）和多个（whitelist_items），
// 多个的语义是「任意一项被禁即拦截」。

// OperationWhitelistItems 是 operation 到限制项的绑定表。
//
// 原项目这些路由挂了白名单（Stage 3 迁移时逐个补进来）：
//
//	GET  /v1/market/purchase                  no_collection_trade
//	POST /v1/market/orders/create/sale        no_collection_trade
//	POST /v1/market/order/create/purchase     no_collection_trade
//	POST /v1/market/purchase/create           no_collection_trade, no_tea_trade, no_tao_trade
//	POST /v1/market/purchase/purchaseBuy      no_collection_trade, no_tea_trade, no_tao_trade
//	POST /v1/market/purchase/grabPriceSubmit  no_collection_trade, no_tea_trade, no_tao_trade
//
// ⚠️ 改 proto 里的 service/method 名时必须同步改这里，否则白名单会静默失效
// （不报错，只是不再拦截）。
var OperationWhitelistItems = map[string][]string{
	"/xtravel.v1.MarketService/PurchaseIndex": {biz.WhitelistItemNoCollectionTrade},
}

// WhitelistMiddleware 构造白名单中间件。
//
// 必须挂在 AuthMiddleware **之内**：它依赖 context 里的用户信息。
// 未登录用户按原实现直接放行（原代码 `if ($userId <= 0) { return $next($request); }`），
// 而实际上这些路由都不是免登录的，所以正常请求一定带 userId。
func WhitelistMiddleware(wl *biz.WhitelistUsecase, logger log.Logger) middleware.Middleware {
	l := log.NewHelper(logger)

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				return handler(ctx, req)
			}
			items := OperationWhitelistItems[tr.Operation()]
			if len(items) == 0 {
				return handler(ctx, req)
			}

			userID := biz.UserIDFromContext(ctx)
			if userID <= 0 {
				return handler(ctx, req)
			}

			// ⚠️ 这里暂时传空的细粒度参数。
			//
			// 原实现把整个 $request->post() 传进 checkPermissionWithContext，
			// 只有 no_tea_trade / no_tao_trade 会读其中的 pay_way / purchase_id / opt_pwd。
			// 而这三个 code 目前只挂在 POST 写接口上（create / purchaseBuy /
			// grabPriceSubmit），那些接口尚未迁移；在中间件里读 POST body 也有
			// 「body 只能读一次」的问题（解码器后面还要用）。
			//
			// 所以：**基础权限判断完全生效**，细粒度部分等写接口迁移时再接，
			// 届时走 http.Context 的 Bind 或让 handler 自己再校验一次。
			// 细粒度逻辑本身已由 biz/whitelist_test.go 单测覆盖。
			var p biz.WhitelistParams

			for _, code := range items {
				blocked, err := wl.CheckPermissionWithContext(ctx, userID, code, p)
				if err != nil {
					return nil, err
				}
				if blocked {
					// 原: JsonService::fail('白名单用户-暂无该操作权限', ['code'=>$itemCode], 0, 1)
					// 注意 data 里带的是 **限制项 code**（字符串），不是数字业务码
					l.WithContext(ctx).Debugf("白名单拦截 user_id=%d item=%s", userID, code)
					return nil, httpx.FailWithData("白名单用户-暂无该操作权限",
						map[string]any{"code": code})
				}
			}
			return handler(ctx, req)
		}
	}
}
