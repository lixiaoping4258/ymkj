package server

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
	"github.com/lixiaoping4258/ymkj/internal/service"
)

// NewHTTPServer 组装 HTTP 服务。
//
// 中间件顺序很关键，从外到内：
//  1. recovery    —— 最外层，保证 panic 不会打挂进程
//  2. logging     —— 记录 method/path/耗时/错误码
//  3. AuthMiddleware —— 鉴权，替代原项目的 LoginMiddleware
//
// Auth 必须在 logging 之内、业务之前：这样鉴权失败也会被 logged，
// 而且业务 handler 一进来就能从 context 拿到 userId。
func NewHTTPServer(
	c *conf.Server,
	auth *conf.Auth,
	common *service.CommonService,
	user *service.UserService,
	market *service.MarketService,
	account *service.AccountService,
	tokens *biz.UserTokenUsecase,
	whitelist *biz.WhitelistUsecase,
	logger log.Logger,
) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			RequestQueryMiddleware(),
			recovery.Recovery(),
			logging.Server(logger),
			AuthMiddleware(tokens, auth, logger),
			// 白名单必须在 Auth 之内：它依赖 context 里的用户信息
			WhitelistMiddleware(whitelist, logger),
		),
		// 关键：换成原项目 JsonService 的信封，否则前端拿到的格式全变
		http.ResponseEncoder(httpx.ResponseEncoder),
		http.ErrorEncoder(httpx.ErrorEncoder),
	}
	if c != nil && c.Http != nil {
		if c.Http.Network != "" {
			opts = append(opts, http.Network(c.Http.Network))
		}
		if c.Http.Addr != "" {
			opts = append(opts, http.Address(c.Http.Addr))
		}
		if c.Http.Timeout != nil {
			opts = append(opts, http.Timeout(c.Http.Timeout.AsDuration()))
		}
	}
	srv := http.NewServer(opts...)
	v1.RegisterCommonServiceHTTPServer(srv, common)
	v1.RegisterAccountServiceHTTPServer(srv, account)
	v1.RegisterUserServiceHTTPServer(srv, user)
	v1.RegisterMarketServiceHTTPServer(srv, market)
	return srv
}
