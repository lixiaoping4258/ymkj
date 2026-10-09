package service

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// AccountService 对应原项目 app/api/controller/v1/account/LoginController.php。
//
// 路由分组是 `Route::group('/v1/login', ...)`，所以路径是 **/v1/login/account**，
// 不是 /account/account。
type AccountService struct {
	v1.UnimplementedAccountServiceServer
	login *biz.LoginUsecase
	val   *biz.LoginValidator
	log   *log.Helper
}

func NewAccountService(login *biz.LoginUsecase, val *biz.LoginValidator, logger log.Logger) *AccountService {
	return &AccountService{login: login, val: val, log: log.NewHelper(logger)}
}

// AccountLogin 对应 LoginController::account。
//
// 原实现（四步，顺序不能变）：
//
//	public function account(): Json {
//	    $params = (new LoginAccountValidate())->post()->goCheck();   // 规则 + checkConfig
//	    $params['app_id'] = 1;
//	    $result = LoginLogic::login($params);
//	    if (false === $result) { return $this->fail(LoginLogic::getError()); }
//	    return $this->data($result);
//	}
//
// ⚠️ 分层对应关系：
//
//	① val.Validate      → $rule（terminal/scene/account）
//	② val.CheckConfig   → checkConfig($scene)（验证码 + 密码 + 锁定）
//	③ login.LoginByAccountPassword → LoginLogic::login
//
// 三步各自的错误信息都会原样变成 {"code":0,"show":1,"msg":"..."}，
// **顺序错了同一个错误请求会返回不同提示**。
func (s *AccountService) AccountLogin(
	ctx context.Context, req *v1.AccountLoginRequest,
) (*v1.AccountLoginReply, error) {
	clientIP := clientIPFrom(ctx)
	avatarPrefix := domainFrom(ctx)

	p := &biz.LoginParams{
		Terminal:  req.GetTerminal(),
		Scene:     req.GetScene(),
		Account:   req.GetAccount(),
		Password:  req.GetPassword(),
		CaptchaID: req.GetCaptchaId(),
		Captcha:   req.GetCaptcha(),
		Code:      req.GetCode(),
	}

	// ① + ② 校验（对应 goCheck）
	if err := s.val.Validate(p); err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}
	if err := s.val.CheckConfig(ctx, p, clientIP); err != nil {
		return nil, bizFail(biz.NewBizError(err.Error()))
	}

	// ③ 登录（本批只支持 scene = ACCOUNT_PASSWORD）
	//
	// 原实现把 defaultAvatar 取自 Config::get('project.default_image.user_avatar')，
	// 实测值 = 'resource/image/adminapi/default/default_avatar.png'。
	const defaultAvatar = "resource/image/adminapi/default/default_avatar.png"

	res, err := s.login.LoginByAccountPassword(ctx, p.Account, p.Terminal, clientIP, defaultAvatar, avatarPrefix)
	if err != nil {
		// LoginLogic::login 内部的 setError 都是可直接展示的业务错误
		return nil, bizFail(biz.NewBizError(err.Error()))
	}

	return &v1.AccountLoginReply{
		Nickname: res.Nickname,
		Sn:       res.Sn,
		Mobile:   res.Mobile,
		Avatar:   res.Avatar,
		Token:    res.Token,
	}, nil
}

/* ------------------------------------------------------------------ 请求上下文提取 */

// clientIPFrom 对应 PHP 的 request()->ip()。
//
// ThinkPHP 的 Request::ip($type = 0) 取值顺序（$type=0 时会查代理头）：
//
//	if (isset($_SERVER['HTTP_X_FORWARDED_FOR'])) { ... }     // 取第一个非 unknown
//	elseif (isset($_SERVER['HTTP_CLIENT_IP'])) { ... }
//	elseif (isset($_SERVER['REMOTE_ADDR'])) { ... }
//
// ⚠️ 这个值直接决定**锁定计数按谁计**，取错会让锁定形同虚设或误伤。
func clientIPFrom(ctx context.Context) string {
	if tr, ok := khttp.RequestFromServerContext(ctx); ok {
		if xff := tr.Header.Get("X-Forwarded-For"); xff != "" {
			// 取第一个非 unknown 的地址（与 ThinkPHP 一致）
			for _, part := range strings.Split(xff, ",") {
				ip := strings.TrimSpace(part)
				if ip != "" && !strings.EqualFold(ip, "unknown") {
					return ip
				}
			}
		}
		if ip := tr.Header.Get("X-Real-IP"); ip != "" {
			return ip
		}
		if ip := tr.Header.Get("Client-Ip"); ip != "" {
			return ip
		}
		return tr.RemoteAddr
	}
	return ""
}

// domainFrom 对应 PHP 的 request()->domain() —— 返回 `scheme://host`。
//
// ⚠️ 只在 `FileService::getFileUrl` 的 **local 驱动**分支用到，但必须从**当前请求**推导，
// 不能用配置里的固定域名，否则头像 URL 与实际访问域名不一致。
func domainFrom(ctx context.Context) string {
	tr, ok := khttp.RequestFromServerContext(ctx)
	if !ok {
		return ""
	}
	host := tr.Host
	if host == "" {
		host = tr.Header.Get("Host")
	}
	scheme := "http"
	if tr.TLS != nil || strings.EqualFold(tr.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}
