package biz

// BizError 表示「可以直接展示给用户的业务错误」。
//
// 对应原项目里的模式：
//
//	GoodsLogic::setError('当前不在兑换时间内…');
//	return false;
//	// 控制器：return $this->fail(GoodsLogic::getError());
//
// 最终会变成 {"code":0,"show":1,"msg":"<这句话>","data":[]}。
//
// 用独立类型而不是普通 error 的原因：biz 层不应该知道 HTTP 信封，
// 由 service 层用 errors.As 把它翻译成 httpx.Fail(...)，
// 其它类型的错误则原样上抛（那才是真的故障，应该走 500 语义）。
type BizError struct {
	Msg string
}

func (e *BizError) Error() string { return e.Msg }

// NewBizError 构造一个会展示给用户的业务错误。
func NewBizError(msg string) error { return &BizError{Msg: msg} }
