package data

import (
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// NewAppFlags 把影响业务分支的环境开关收拢到一个显式结构里。
//
// 为什么不用散落的 bool 参数：这个开关在原项目里叫 APP_DEBUG，
// 名字看着只是"调试"，实际上 PurchaseController::index 和
// PurchaseFaceLists::queryWhere 都用它决定走不走"生产测试用户"分支
// （那批用户会跳过交易时段判断、并且不过滤 AppArchive.state）。
// 命名和影响范围完全不匹配，所以这里显式建模并加了注释。
//
// 默认值取 false（与原项目 env('APP_DEBUG') 缺省时一致），
// 也就是**默认按生产行为**，避免漏配时把测试分支带上线。
func NewAppFlags(c *conf.App) *biz.AppFlags {
	debug := false
	if c != nil {
		debug = c.Debug
	}
	return &biz.AppFlags{AppDebug: debug}
}
