//go:build wireinject
// +build wireinject

// 这个文件只在 wire 生成依赖注入代码时参与编译（见 wireinject 构建标签）。
// 生成命令：wire ./cmd/xtravel   （或 make wire）
// 产物：cmd/xtravel/wire_gen.go —— 那个文件要提交到仓库。

package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/data"
	"github.com/lixiaoping4258/ymkj/internal/server"
	"github.com/lixiaoping4258/ymkj/internal/service"
)

// wireApp 由 wire 生成实现。
//
// 依赖装配顺序：conf -> data(DB/Redis/Repo) -> biz(Usecase) -> service -> server -> App
// 对应原项目里那些 static 单例和 think 容器的自动解析，但这里是编译期确定的，
// 少一个依赖会直接编译不过，而不是运行时报错。
func wireApp(
	*conf.Server,
	*conf.Data,
	*conf.App,
	*conf.Log,
	log.Logger,
) (*kratos.App, func(), error) {
	panic(wire.Build(
		server.ProviderSet,
		data.ProviderSet,
		biz.ProviderSet,
		service.ProviderSet,
		newApp,
	))
}
