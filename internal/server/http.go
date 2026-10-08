package server

import (
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/transport/http"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
	"github.com/lixiaoping4258/ymkj/internal/service"
)

// NewHTTPServer 组装 HTTP 服务。
//
// 三个关键点：
//  1. ResponseEncoder / ErrorEncoder 必须换成 httpx 的实现，
//     否则响应信封不是原项目的 {code,show,msg,data}，前端全部解析失败。
//  2. recovery 放最外层，保证 panic 不会打挂进程（原项目有全局异常处理）。
//  3. logging.Server 记录 method/path/耗时，替代原项目塞在 log_path 里的访问日志。
func NewHTTPServer(c *conf.Server, common *service.CommonService, logger log.Logger) *http.Server {
	var opts = []http.ServerOption{
		http.Middleware(
			recovery.Recovery(),
			logging.Server(logger),
		),
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
	return srv
}
