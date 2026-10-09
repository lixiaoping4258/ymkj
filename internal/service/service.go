package service

import (
	"errors"

	"github.com/google/wire"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
)

// ProviderSet 是 service 层的依赖注入集合。
var ProviderSet = wire.NewSet(NewCommonService, NewUserService, NewMarketService, NewAccountService, NewArticleService)

// bizFail 把 biz 层的业务错误翻译成原项目的 fail 信封，其它错误原样上抛。
//
// 对应原项目控制器里反复出现的这段：
//
//	if ($data === false) {
//	    return $this->fail(Logic::getError());
//	}
//
// 也就是 {"code":0,"show":1,"msg":"<业务提示>","data":[]}。
//
// 为什么不直接在 biz 里返回 httpx.Fail：那会让 biz 层依赖 HTTP 信封，
// 分层就烂了。biz 只负责抛 *biz.BizError，翻译是 service 的职责。
//
// 非 BizError 的错误原样返回，走 Kratos 的错误链 → 上层能看到真实原因，
// 而不是被包装成一句给用户看的提示。
func bizFail(err error) error {
	if err == nil {
		return nil
	}
	var be *biz.BizError
	if errors.As(err, &be) {
		return httpx.Fail(be.Msg)
	}
	return err
}
