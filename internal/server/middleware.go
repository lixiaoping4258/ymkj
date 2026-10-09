package server

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
)

// 本文件对应原项目 app/api/http/middleware/LoginMiddleware.php。
//
// 逐条对齐的行为：
//  1. 读 header `token`
//  2. 该操作是否免登录（原项目是控制器里的 $notNeedLogin 数组）
//  3. 无 token 且需要登录 -> code=-403 show=0 "请求认证信息有误，请重新登录"
//  4. token 无效/过期且需要登录 -> code=-403 show=0 "登录超时，请重新登录"
//  5. 临近过期自动续期；续期失败 -> code=-403 show=1 "登录过期"
//  6. 把用户信息塞进 context 供 service 层使用
//
// ⚠️ 两个必须保持的细节：
//   - 失败用的业务码是 **-403**（不是 0），且前两个 show=0 —— 前端靠它
//     决定"不弹提示，直接跳登录页"。改成 0 或 show=1 都会让前端行为变化。
//   - HTTP 状态码仍是 200（由 httpx.ErrorEncoder 统一处理）。

// defaultPublicOps 是默认免登录白名单。
//
// 原项目是在每个控制器上声明 $notNeedLogin，Kratos 里没有"控制器对象"这个概念，
// 所以展开成 operation 全路径。这些路径由 proto 的 package.service/method 决定，
// 改名时要同步改这里。
//
// ⚠️⚠️ 逐个对着 PHP 的 $notNeedLogin 数组核过，**不要凭接口名猜是否公开**：
//
//	app/api/controller/v1/common/ConfigController.php  $notNeedLogin = ['index']
//	    -> GetConfig          免登录 ✔
//	app/api/controller/IndexController.php             $notNeedLogin = ['test','index','config','policy','decorate']
//	    -> GetTradeConfig     **不在列表里 -> 需要登录**（Stage 1 曾误标为公开）
//	app/api/controller/v1/user/UserController.php      $notNeedLogin = ['resetPassword']
//	    -> GetUserInfo        需要登录 ✔
//	app/api/controller/v1/market/PurchaseController.php $notNeedLogin = ['index']
//	    -> PurchaseIndex      免登录 ✔
//	    -> CheckExchange / GetPayWay  需要登录 ✔
//
// 标错的后果：
//   - 把该登录的标成公开 -> 未授权访问（安全）
//   - 把公开的标成需登录 -> 前端匿名访问直接 403（功能不可用，本次踩的就是这个）
var defaultPublicOps = map[string]struct{}{
	"/xtravel.v1.CommonService/GetConfig":         {},
	"/xtravel.v1.MarketService/PurchaseIndex":     {},
	"/xtravel.v1.MarketService/GetSaleCategories": {},
	// ⚠️ 不要凭接口名猜！这一条来自：
	//   app/api/controller/v1/account/LoginController.php  $notNeedLogin 含 'account'
	//   路由：POST /v1/login/account（account.php 里 Route::group('/v1/login', ...)）
	// 见 docs/ROUTES-AND-AUTH.md 的免登录清单。
	"/xtravel.v1.AccountService/AccountLogin": {},
	// CaptchaController::$notNeedLogin = ['index','captcha']
	"/xtravel.v1.CaptchaService/GetCaptcha": {},
	// IndexController::$notNeedLogin 含 'policy'（路由是 /v1/common/protocol）
	"/xtravel.v1.CommonService/GetProtocol": {},
}

// AuthMiddleware 构造鉴权中间件。
func AuthMiddleware(tokens *biz.UserTokenUsecase, c *conf.Auth, logger log.Logger) middleware.Middleware {
	l := log.NewHelper(logger)

	public := make(map[string]struct{}, len(defaultPublicOps))
	for k := range defaultPublicOps {
		public[k] = struct{}{}
	}
	// 配置里的白名单是**追加**而不是替换：配置漏写时不能把所有接口锁死
	if c != nil {
		for _, op := range c.PublicOperations {
			public[op] = struct{}{}
		}
	}

	// 提前算好续期窗口，避免每个请求都做一次类型转换
	beExpire := 3600
	if c != nil && c.BeExpireDuration > 0 {
		beExpire = int(c.BeExpireDuration)
	}

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			var token, operation string
			if tr, ok := transport.FromServerContext(ctx); ok {
				token = tr.RequestHeader().Get("token")
				operation = tr.Operation()
			}
			_, isPublic := public[operation]

			// 步骤 3：没带 token 且这个接口要登录
			if token == "" && !isPublic {
				return nil, httpx.FailCode(-403, 0, "请求认证信息有误，请重新登录")
			}

			info, err := tokens.GetUserInfo(ctx, token)
			if err != nil {
				return nil, err
			}

			// 步骤 4：token 无效或已过期且这个接口要登录
			if (info == nil || info.UserID == 0) && !isPublic {
				return nil, httpx.FailCode(-403, 0, "登录超时，请重新登录")
			}

			// 步骤 5：临近过期自动续期（原实现只在 $userInfo 非空时做）
			if info != nil && info.UserID != 0 {
				if time.Now().Unix() > info.ExpireTime-int64(beExpire) {
					renewed, rerr := tokens.OvertimeToken(ctx, token)
					if rerr != nil {
						return nil, rerr
					}
					if renewed == nil {
						// 原: JsonService::fail('登录过期', [], -403) —— 这个 show 是默认值 1
						return nil, httpx.FailCode(-403, 1, "登录过期")
					}
					info = renewed
					l.Debugf("token 已自动续期 user_id=%d operation=%s", info.UserID, operation)
				}
			}

			// 步骤 6：注入 context
			if info != nil {
				ctx = biz.WithUserInfo(ctx, info)
			}
			return handler(ctx, req)
		}
	}
}
