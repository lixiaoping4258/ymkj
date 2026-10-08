package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// UserService 对应原项目 app/api/controller/v1/user/UserController.php。
type UserService struct {
	v1.UnimplementedUserServiceServer
	users *biz.UserUsecase
}

func NewUserService(users *biz.UserUsecase) *UserService {
	return &UserService{users: users}
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
		Id:          profile.ID,
		Sn:          profile.Sn,
		Sex:         profile.Sex,
		Nickname:    profile.Nickname,
		RealName:    profile.RealName,
		Avatar:      profile.Avatar,
		Mobile:      profile.Mobile,
		CreateTime:  profile.CreateTime,
		UserMoney:   profile.UserMoney,
		OptPwd:      profile.OptPwd,
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
