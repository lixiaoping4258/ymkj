package main

import (
	"flag"
	"os"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport/http"

	// 配置文件的编解码器：yaml 用来读 configs/config.yaml
	_ "github.com/go-kratos/kratos/v2/encoding/json"
	_ "github.com/go-kratos/kratos/v2/encoding/yaml"

	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// 由构建时注入，见 Makefile 的 -ldflags
var (
	Name     = "xtravel"
	Version  = "v0.1.0"
	flagconf string
	id, _    = os.Hostname()
)

func init() {
	flag.StringVar(&flagconf, "conf", "configs/config.yaml", "配置文件路径，例如 -conf configs/config.yaml")
}

// newLogger 按配置组装 logger。
//
// level 用 Kratos 的 FilterLevel 过滤；format=json 时输出结构化日志，
// 便于接 ELK（原项目是把日志写到 runtime/log 再由 filebeat 采集）。
func newLogger(lc *conf.Log) log.Logger {
	var l log.Logger = log.NewStdLogger(os.Stdout)
	l = log.With(l,
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", id,
		"service.name", Name,
		"service.version", Version,
	)
	if lc == nil {
		return l
	}
	var lv log.Level
	switch lc.Level {
	case "debug":
		lv = log.LevelDebug
	case "warn":
		lv = log.LevelWarn
	case "error":
		lv = log.LevelError
	default:
		lv = log.LevelInfo
	}
	return log.NewFilter(l, log.FilterLevel(lv))
}

func newApp(logger log.Logger, hs *http.Server) *kratos.App {
	// kratos.New 默认监听 SIGTERM/SIGINT 做优雅退出 ——
	// 对应 supervisor 配置里的 stopsignal=INT：会先停止接收新请求，
	// 等在途请求处理完再退出，而不是被硬杀。
	return kratos.New(
		kratos.ID(id),
		kratos.Name(Name),
		kratos.Version(Version),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs),
	)
}

func main() {
	flag.Parse()

	c := config.New(config.WithSource(file.NewSource(flagconf)))
	defer func() { _ = c.Close() }()

	if err := c.Load(); err != nil {
		panic(err)
	}

	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}

	logger := newLogger(bc.Log)
	helper := log.NewHelper(logger)
	helper.Infof("%s %s 启动中，配置来源: %s", Name, Version, flagconf)

	app, cleanup, err := wireApp(bc.Server, bc.Data, bc.App, bc.Log, bc.Auth, logger)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	if err := app.Run(); err != nil {
		panic(err)
	}
}
