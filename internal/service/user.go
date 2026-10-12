package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/httpx"
	"github.com/lixiaoping4258/ymkj/internal/pkg/pbconv"
)

// UserService 对应原项目 app/api/controller/v1/user/UserController.php。
type UserService struct {
	v1.UnimplementedUserServiceServer
	users       *biz.UserUsecase
	noticeNew   *biz.NoticeNewUsecase
	address     *biz.AddressUsecase
	addressList *biz.AddressListUsecase
	realInfo    *biz.RealInfoUsecase
	unreg       *biz.UnregisterCheckUsecase
	msgRead     *biz.MessageReadUsecase
	msgDetail   *biz.MessageDetailUsecase
	addrDel     *biz.AddressDeleteUsecase
}

func NewUserService(users *biz.UserUsecase, noticeNew *biz.NoticeNewUsecase, address *biz.AddressUsecase, addressList *biz.AddressListUsecase, realInfo *biz.RealInfoUsecase, unreg *biz.UnregisterCheckUsecase, msgRead *biz.MessageReadUsecase, msgDetail *biz.MessageDetailUsecase, addrDel *biz.AddressDeleteUsecase) *UserService {
	return &UserService{users: users, noticeNew: noticeNew, address: address, addressList: addressList, realInfo: realInfo, unreg: unreg, msgRead: msgRead, msgDetail: msgDetail, addrDel: addrDel}
}

// GetUserInfo 对应 UserController::info。
//
// 原实现：
//
//	public function info(): Json {
//	    $result = UserLogic::info($this->userId);
//	    return $this->data($result);
//	}
//
// $this->userId 由 LoginMiddleware 从 token 解出来后挂在 request 上；
// 这里对应从 context 里取（biz.UserIDFromContext）。
func (s *UserService) GetUserInfo(ctx context.Context, _ *v1.GetUserInfoRequest) (*v1.GetUserInfoReply, error) {
	userID := biz.UserIDFromContext(ctx)
	profile, err := s.users.Info(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		// 极罕见的竞态：鉴权通过后用户行被删。原实现会返回空数组，
		// 这里返回零值对象，字段齐全但都是空 —— 前端不会崩。
		return &v1.GetUserInfoReply{}, nil
	}
	return &v1.GetUserInfoReply{
		Id:         profile.ID,
		Sn:         profile.Sn,
		Sex:        profile.Sex,
		Nickname:   profile.Nickname,
		RealName:   profile.RealName,
		Avatar:     profile.Avatar,
		Mobile:     profile.Mobile,
		CreateTime: profile.CreateTime,
		// 可空字段用 StringValue：nil -> null，非 nil -> 字符串。
		// 直接赋 string 会把 NULL 变成 ""，与 PHP 的 null 不一致
		// （实测 1365 个用户里 1277 个 opt_pwd 是 NULL）。
		UserMoney:   pbconv.StringValue(profile.UserMoney),
		OptPwd:      pbconv.StringValue(profile.OptPwd),
		HasPassword: profile.HasPassword,
		HasOptPwd:   profile.HasOptPwd,
		IsReal:      profile.IsReal,
		UserCode:    profile.UserCode,
		UserCodeAuth: &v1.UserCodeAuth{
			TeaUserCode: profile.UserCodeAuth.TeaUserCode,
			TaoUserCode: profile.UserCodeAuth.TaoUserCode,
		},
	}, nil
}

// GetUserNoticeNew 对应 v1/user/NoticeController::getNew。
//
// 原实现：
//
//	$getNew = NoticeLogic::getNew();
//	return $this->data(['get_new' => $getNew]);      // ⚠️ 外面套了 get_new
//
// ⚠️ `get_new` 的值**要么是对象（文章行），要么是空数组 []** ——
//
//	源码里是 `$article->isEmpty() ? [] : $article->toArray()`。
//	两种形态用同一个 proto 消息表达不了（EmitUnpopulated 会把缺省字段补出来），
//	所以用 RawData 自己拼 JSON。
//
// ⚠️ 免登录（v1/user/NoticeController::$notNeedLogin = ['getNew']）。
func (s *UserService) GetUserNoticeNew(
	ctx context.Context, _ *v1.GetUserNoticeNewRequest,
) (*v1.RawData, error) {
	body, err := s.noticeNew.GetNew(ctx)
	if err != nil {
		return nil, err
	}
	// 拼成 {"get_new": <body>}；body 本身就是合法 JSON（对象或 []）
	out := make([]byte, 0, len(body)+16)
	out = append(out, `{"get_new":`...)
	out = append(out, body...)
	out = append(out, '}')
	return &v1.RawData{Json: out}, nil
}

// GetAddressDefault 对应 v1/user/AddressController::addressDefault。
//
// 原实现：
//
//	$params = request()->get();
//	$result = AddressLogic::addressDefault($params, $this->userId);
//	if ($result) { return $this->success('成功', $result); }
//	else { return $this->fail(AddressLogic::getError()); }
//
// ⚠️ 用的是 `success('成功', ...)` —— **不是 data()**。
//
//	BaseLikeAdminController::success 的默认 msg 是 'success'，
//	这里显式传了 '成功'，所以响应里 **msg 是 "成功"**（其他接口是空串）。
//	这个差异写在 GetAddressDefaultReply.Envelope() 里。
func (s *UserService) GetAddressDefault(
	ctx context.Context, req *v1.GetAddressDefaultRequest,
) (*v1.GetAddressDefaultReply, error) {
	userID := biz.UserIDFromContext(ctx)
	row, err := s.address.AddressDefault(ctx, uint32(userID), req.GetId())
	if err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	return &v1.GetAddressDefaultReply{
		Id: row.ID, IsDefault: row.IsDefault, Address: row.Address,
	}, nil
}

// GetAddressList 对应 v1/user/AddressController::index。
//
// 原实现：`return $this->dataLists(new AddressLists());`
//
// ⚠️ 这条**需登录**：AddressController 未声明 $notNeedLogin。
// ⚠️ 内部是 INNER JOIN x_app，见 proto 注释。
func (s *UserService) GetAddressList(
	ctx context.Context, _ *v1.GetAddressListRequest,
) (*v1.RawData, error) {
	userID := biz.UserIDFromContext(ctx)
	p := biz.ParsePageParams(httpx.QueryFrom(ctx))
	ld, err := s.addressList.List(ctx, uint32(userID), p)
	if err != nil {
		return nil, err
	}
	raw, err := ld.ToJSON()
	if err != nil {
		return nil, err
	}
	return &v1.RawData{Json: raw}, nil
}

// GetRealInfo 对应 UserController::getRealInfo。
//
// 原实现：
//
//	$info = CardRealLogic::getUserRealInfo($this->userId);
//	if($info === false) { return $this->fail(CardRealLogic::getError()); }
//	return $this->data($info);
func (s *UserService) GetRealInfo(
	ctx context.Context, _ *v1.GetRealInfoRequest,
) (*v1.GetRealInfoReply, error) {
	userID := biz.UserIDFromContext(ctx)
	res, err := s.realInfo.GetUserRealInfo(ctx, uint32(userID))
	if err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	reply := &v1.GetRealInfoReply{
		State:    res.State,
		CardNo:   res.CardNo,
		UserName: res.UserName,
	}
	if res.Birthday != nil {
		reply.Birthday = wrapperspb.String(*res.Birthday)
	}
	if res.Gender != nil {
		reply.Gender = wrapperspb.Int32(*res.Gender)
	}
	if res.Address != nil {
		reply.Address = wrapperspb.String(*res.Address)
	}
	return reply, nil
}

// GetUnregisterCheck 对应 UserController::unregisterCheck。
//
// 原实现：
//
//	$result = UserLogic::unregisterCheck($this->userId);
//	return $this->success('请求成功', $result);     // ⚠️ 不是 data()
func (s *UserService) GetUnregisterCheck(
	ctx context.Context, _ *v1.GetUnregisterCheckRequest,
) (*v1.GetUnregisterCheckReply, error) {
	userID := biz.UserIDFromContext(ctx)
	res, err := s.unreg.UnregisterCheck(ctx, uint32(userID))
	if err != nil {
		return nil, err
	}
	words := res.ErrWord
	if words == nil {
		words = []string{}
	}
	return &v1.GetUnregisterCheckReply{State: res.State, ErrWord: words}, nil
}

// SetMessageAllRead 对应 MessageController::setAllRead（POST /v1/user/message/setAllRead）。
//
// 原实现：
//
//	$data = MessageLogic::setAllRead($this->userId);
//	if ($data === false) { return $this->fail(MessageLogic::getError()); }
//	return $this->data([]);          // ⚠️ data 是**空数组**
func (s *UserService) SetMessageAllRead(
	ctx context.Context, _ *v1.SetMessageAllReadRequest,
) (*v1.RawData, error) {
	userID := biz.UserIDFromContext(ctx)
	if err := s.msgRead.SetAllRead(ctx, uint32(userID)); err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	// 对应 `$this->data([])` —— 空数组，不是 null 也不是 {}
	return &v1.RawData{Json: []byte("[]")}, nil
}

// SetMessageOneRead 对应 MessageController::setOneRead（POST /v1/user/message/setOneRead）。
//
// 原实现：
//
//	$params = (new MessageIdValidate())->post()->goCheck();   // id: require|integer|gt:0
//	$data = MessageLogic::setOneRead($this->userId, $params);
//	if ($data === false) { return $this->fail(MessageLogic::getError()); }
//	return $this->data([]);
//
// ⚠️ 校验失败时（id 缺失 / 非整数 / <=0）应当返回校验器的错误信息。
//
//	这里按规则手工复刻：id <= 0 视为校验失败。
//	TODO: 精确的错误文案需读 BaseValidate 的默认 message（尚未确认），
//	当前返回的是框架通用文案。**这一点在接入前需要补**。
func (s *UserService) SetMessageOneRead(
	ctx context.Context, req *v1.SetMessageOneReadRequest,
) (*v1.RawData, error) {
	// 逐字对应 MessageIdValidate 的三条文案，并复刻它们的**判定顺序**
	// （require -> integer -> gt:0，返回第一个失败的）：
	//   'id.require' => '消息ID不能为空'
	//   'id.integer' => '消息ID不正确'
	//   'id.gt'      => '消息ID必须大于0'
	//
	// ⚠️ `integer` 这条在 Go 侧由 protojson 的解码承担（非整数会解码失败），
	//    所以这里不需要再判；但文案不同，接前若发现前端依赖该文案需另作处理。
	if req.Id == nil {
		return nil, bizFail(biz.NewBizError("消息ID不能为空"))
	}
	if req.Id.Value <= 0 {
		return nil, bizFail(biz.NewBizError("消息ID必须大于0"))
	}
	userID := biz.UserIDFromContext(ctx)
	if err := s.msgRead.SetOneRead(ctx, uint32(userID), req.Id.Value); err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	return &v1.RawData{Json: []byte("[]")}, nil
}

// GetMessageDetail 对应 MessageController::detail（GET /v1/user/message/detail?id=N）。
//
// 原实现：
//
//	$params = (new MessageIdValidate())->get()->goCheck();   // id: require|integer|gt:0
//	$data = MessageLogic::detail($this->userId, $params);
//	if ($data === false) { return $this->fail(MessageLogic::getError()); }
//	return $this->data($data);
func (s *UserService) GetMessageDetail(
	ctx context.Context, req *v1.GetMessageDetailRequest,
) (*v1.RawData, error) {
	// 与 setOneRead 同一套校验文案（同一个 Validate 类）
	if req.Id == nil {
		return nil, bizFail(biz.NewBizError("消息ID不能为空"))
	}
	if req.Id.Value <= 0 {
		return nil, bizFail(biz.NewBizError("消息ID必须大于0"))
	}
	userID := biz.UserIDFromContext(ctx)
	res, err := s.msgDetail.Detail(ctx, uint32(userID), req.Id.Value)
	if err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	row := res.Row
	// 键序按原文 field() 的顺序：m.id, m.send_id, m.recv_id, m.message_id,
	// m.is_read, m.create_time, mb.type, mb.title, mb.body, mb.flag，最后 type_name
	body, merr := biz.MarshalNoHTMLEscape(map[string]any{
		"id":          row.ID,
		"send_id":     derefI64Any(row.SendID),
		"recv_id":     derefI64Any(row.RecvID),
		"message_id":  derefI64Any(row.MessageID),
		"is_read":     row.IsRead,
		"create_time": row.CreateTime,
		"type":        row.Type,
		"title":       row.Title,
		"body":        row.Body,
		"flag":        row.Flag,
		"type_name":   res.TypeName,
	})
	if merr != nil {
		return nil, merr
	}
	return &v1.RawData{Json: body}, nil
}

// derefI64Any 把 *int64 转成 any：nil -> null（PHP 对 NULL 列输出 null）
func derefI64Any(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// DeleteAddress 对应 AddressController::delete（GET /v1/user/address/delete?id=N）。
//
// ⚠️⚠️ **这是一个用 GET 实现的删除接口**，而且删除是**硬删除**（表无 delete_time）。
//
//	迁移后要确保：
//	  - 网关/浏览器不要预取这个 URL（否则会被提前删掉）
//	  - 前端不要把它当成可缓存的 GET
//
// 原实现：
//
//	$params = (new UserAddressDeleteValidate())->get()->goCheck();
//	$result = AddressLogic::delete($params['id'], $this->userId);
//	if ($result) { return $this->success('删除成功'); }      // ⚠️ 不是 data()
//	else { return $this->fail(AddressLogic::getError()); }
func (s *UserService) DeleteAddress(
	ctx context.Context, req *v1.DeleteAddressRequest,
) (*v1.DeleteAddressReply, error) {
	// 校验文案逐字对应 UserAddressDeleteValidate：
	//   'id.require' => '地址记录ID不能为空'
	//   'id.number'  => '地址记录ID不正确'
	// ⚠️ 规则是 require|number（不是 integer|gt:0）—— 所以 **0 是合法的**，
	//    会走到"未找到地址记录"。这里不能像 setOneRead 那样判 <=0。
	if req.Id == nil {
		return nil, bizFail(biz.NewBizError("地址记录ID不能为空"))
	}
	userID := biz.UserIDFromContext(ctx)
	if err := s.addrDel.Delete(ctx, req.Id.Value, uint32(userID)); err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	// 响应体由 DeleteAddressReply.RawBody() 给出（msg="删除成功"）
	return &v1.DeleteAddressReply{}, nil
}
