package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/account/LoginController::account
//   app/api/logic/LoginLogic::login / register
//   app/api/validate/LoginAccountValidate::checkPassword
//
// ⚠️ 分层必须与原实现一致：**密码校验在"验证器"层，不在 login() 里**。
// 原实现里 LoginAccountValidate::checkPassword 先比对密码，通过后才调 LoginLogic::login。
// 把密码校验塞进本用例会改变错误信息产生的位置与顺序。
//
// 本批只实现 scene = ACCOUNT_PASSWORD（账号密码）。手机验证码 / 第三方登录暂不做。

// 对应 app\common\enum\LoginEnum
//
// ⚠️ 这三个值与 UserAccounts.type（1/2/3）**数值恰好相同但含义不同**，
// 不要混用同一个常量。
const (
	// LoginSceneAccountPassword 账号密码（LoginEnum::ACCOUNT_PASSWORD）
	LoginSceneAccountPassword = 1
	// LoginSceneMobileCaptcha 手机验证码（LoginEnum::MOBILE_CAPTCHA）—— 本批未实现
	LoginSceneMobileCaptcha = 2
	// LoginSceneThirdLogin 第三方登录（LoginEnum::THIRD_LOGIN）—— 本批未实现
	LoginSceneThirdLogin = 3
)

// UserAccounts.type 的取值（与上面前三个数值相同，但语义是"账号类型"）
const (
	AccountTypeAccount = 1 // 账号
	AccountTypeMobile  = 2 // 手机号
	AccountTypeThird   = 3 // 唯一ID / 第三方
)

// ErrLoginAccountDisabled 对应 setError('账号当前不可用')
var ErrLoginAccountDisabled = errors.New("账号当前不可用")

// ErrLoginUserNotFound 对应 setError('不存在的用户信息')
var ErrLoginUserNotFound = errors.New("不存在的用户信息")

// ErrLoginUserFrozen 对应 setError('账号冻结中，无法登录')
var ErrLoginUserFrozen = errors.New("账号冻结中，无法登录")

// ErrLoginUnsupportedScene 对应 setError('不支持的登录方式')
var ErrLoginUnsupportedScene = errors.New("不支持的登录方式")

// UserAccountRow 对应 x_user_accounts 的一行（只取本链路用到的列）。
type UserAccountRow struct {
	ID     uint64
	UserID uint64
	// State 只有 'ENABLE' 才允许登录
	State string
}

// UserLoginRow 是登录链路需要的用户列。
type UserLoginRow struct {
	ID        uint64
	Nickname  string
	Sn        uint64
	Mobile    string
	Avatar    string
	IsDisable int32
}

// RegisterParams 对应 LoginLogic::register 的入参。
type RegisterParams struct {
	// Account 账号
	Account string
	// PlainPassword 明文密码。原实现里 login 走自动注册时传的是 Id::uuid() 生成的随机值
	PlainPassword string
	// Channel 对应 $params['channel']（terminal）
	Channel int32
	// Mobile 手机号，账号密码场景为空
	Mobile string
	// AccountType 对应 $accountType（1/2/3）
	AccountType int
	// AppID 对应 $app_id，非第三方场景为 1
	AppID int
	// ExtData 第三方登录才有的数据（avatar/nickname/app_id）
	ExtData map[string]any
}

// LoginRepo 登录链路的持久化端口。
type LoginRepo interface {
	// FindAccount 对应 UserAccounts::where($condition)->findOrEmpty()
	FindAccount(ctx context.Context, appID, accountType int, account string) (*UserAccountRow, bool, error)
	// Register 对应 LoginLogic::register —— **多表写入，必须在一个事务里**
	// （User + UserAccounts），返回创建出的 UserAccounts 行。
	Register(ctx context.Context, p RegisterParams) (*UserAccountRow, error)
	// FindUserByID 对应 User::where(['id'=>$userId])->findOrEmpty()
	// 注意：PHP 侧带 ThinkPHP 的隐式软删除过滤
	FindUserByID(ctx context.Context, id uint64) (*UserLoginRow, error)
	// UpdateLoginInfo 对应 $user->login_time = time(); $user->login_ip = ip(); $user->save()
	UpdateLoginInfo(ctx context.Context, userID uint64, loginTime int64, loginIP string) error
	// CreateAppNotify 对应 OpenAppsNotifyLogic::create（ThirdNotify 监听器最终做的写库）
	CreateAppNotify(ctx context.Context, action string, appID string, userID uint64, body map[string]any) error
}

// LoginResult 是 login() 的返回值。
//
// 原实现：
//
//	return ['nickname'=>$userInfo['nickname'], 'sn'=>$userInfo['sn'],
//	        'mobile'=>$userInfo['mobile'], 'avatar'=>$avatar, 'token'=>$userInfo['token']];
//
// ⚠️ mobile 取自 **$userInfo（token 缓存里的值）**，不是 $user。
type LoginResult struct {
	Nickname string
	Sn       uint64
	Mobile   string
	Avatar   string
	Token    string
}

// LoginUsecase 登录用例。
type LoginUsecase struct {
	repo   LoginRepo
	tokens *UserTokenUsecase
	ids    IDGenerator
	// avatarPrefix 对应 FileService::getFileUrl 的域名拼接。
	//
	// ⚠️ 原实现里 local 驱动下 $domain = request()->domain()，**取自当前请求**，
	// 不是配置里的固定域名。所以这里传入的是 handler 从 Host 头推导出的值。
	avatarPrefix string
	log          *log.Helper
}

func NewLoginUsecase(
	repo LoginRepo, tokens *UserTokenUsecase, ids IDGenerator,
	avatarPrefix string, logger log.Logger,
) *LoginUsecase {
	return &LoginUsecase{
		repo: repo, tokens: tokens, ids: ids,
		avatarPrefix: avatarPrefix, log: log.NewHelper(logger),
	}
}

// LoginByAccountPassword 对应 scene = ACCOUNT_PASSWORD(1) 且**密码已在验证器层校验通过**。
//
// 原实现流程（LoginLogic::login，已按源码逐行核对）：
//
//	$condition = ['app_id'=>1, 'type'=>1, 'account'=>$params['account']];
//	$account = UserAccounts::where($condition)->findOrEmpty();
//	if ($account->isEmpty()) {
//	    $registerParams = ['channel'=>$params['terminal'], 'account'=>$params['account'],
//	                       'password'=>Id::uuid()];
//	    $result = self::register($registerParams, 1, '');
//	    if ($result === false) { return false; }          // register 内部已 setError
//	    $userId = $result['user_id'];
//	} else {
//	    if ($account->state != 'ENABLE') { self::setError('账号当前不可用'); return false; }
//	    $userId = $account->user_id;
//	}
//	$user = User::where(['id'=>$userId])->findOrEmpty();
//	if ($user->isEmpty()) { self::setError('不存在的用户信息'); return false; }
//	if ($user->is_disable == 1) { self::setError('账号冻结中，无法登录'); return false; }
//	$user->login_time = time(); $user->login_ip = request()->ip(); $user->save();
//	$userInfo = UserTokenService::setToken($user->id, $params['terminal']);
//	$avatar = $user->avatar ?: Config::get('project.default_image.user_avatar');
//	$avatar = FileService::getFileUrl($avatar);
//	// app_id == 2 时触发 ThirdNotify
//	return ['nickname'=>..., 'sn'=>..., 'mobile'=>..., 'avatar'=>$avatar, 'token'=>...];
func (uc *LoginUsecase) LoginByAccountPassword(
	ctx context.Context, account string, terminal int32, clientIP, defaultAvatar string,
) (*LoginResult, error) {
	const appID = 1 // 账号密码场景固定 app_id = 1

	// 1. 查账号
	acc, found, err := uc.repo.FindAccount(ctx, appID, AccountTypeAccount, account)
	if err != nil {
		return nil, err
	}

	var userID uint64
	isCreate := false

	if !found {
		// 2. 账号不存在 -> 自动注册
		//
		// ⚠️ 原实现这里传的密码是 **Id::uuid() 生成的随机值**，不是用户输入。
		// 也就是说"账号密码登录"在账号不存在时会建一个随机密码的账号。
		randomPwd, err := uc.ids.Gen(ctx, 0)
		if err != nil {
			return nil, err
		}
		created, err := uc.repo.Register(ctx, RegisterParams{
			Account:       account,
			PlainPassword: fmt.Sprintf("%d", randomPwd), // 对应 Id::uuid()
			Channel:       terminal,
			Mobile:        "",
			AccountType:   AccountTypeAccount,
			AppID:         appID,
		})
		if err != nil {
			// 原实现里 register 内部已 setError('用户注册失败')，这里不覆盖
			return nil, err
		}
		userID = created.UserID
		isCreate = true
	} else {
		// 3. 账号存在 -> 检查账号状态
		if acc.State != "ENABLE" {
			return nil, ErrLoginAccountDisabled
		}
		userID = acc.UserID
	}

	// 4. 查用户
	u, err := uc.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrLoginUserNotFound
	}
	if u.IsDisable == 1 {
		return nil, ErrLoginUserFrozen
	}

	// 5. 更新登录信息
	if err := uc.repo.UpdateLoginInfo(ctx, u.ID, time.Now().Unix(), clientIP); err != nil {
		return nil, err
	}

	// 6. 签发 token
	//
	// 原实现：$userInfo = UserTokenService::setToken($user->id, $params['terminal']);
	// → 已迁：UserTokenUsecase.SetToken（Stage 2 就实现了，本批终于接上）
	info, err := uc.tokens.SetToken(ctx, u.ID, terminal)
	if err != nil {
		return nil, err
	}
	if info == nil {
		return nil, ErrLoginUserNotFound
	}

	// 7. 头像：$user->avatar ?: config('project.default_image.user_avatar')
	avatar := u.Avatar
	if avatar == "" {
		avatar = defaultAvatar
	}
	avatar = joinFileURL(uc.avatarPrefix, avatar)

	// 8. 条件副作用：app_id == 2 时记一条三方通知
	//
	// ⚠️ 账号密码场景 app_id 恒为 1，**走不到这个分支**。保留是因为第三方登录会复用本函数。
	if appID == 2 {
		msg := "用户登录成功"
		if isCreate {
			msg = "用户注册成功"
		}
		if err := uc.repo.CreateAppNotify(ctx, "account.register", "2", u.ID, map[string]any{
			"err":  "OK",
			"data": map[string]any{"mobile": u.Mobile, "uid": u.ID, "custom_uid": account},
			"msg":  msg,
		}); err != nil {
			// 原实现：handle 返回 false 只记日志，**不影响登录结果**
			uc.log.WithContext(ctx).Warnf("三方通知入库失败 user_id=%d: %v", u.ID, err)
		}
	}

	// 9. 返回。⚠️ mobile / nickname / sn 取自 token 缓存（$userInfo），不是 $user
	// nickname / sn / mobile 取自 $userInfo（token 缓存里的值），不是 $user。
	// 原实现就是这么写的，照抄以保证行为一致。
	return &LoginResult{
		Nickname: info.Nickname,
		Sn:       info.Sn,
		Mobile:   info.Mobile,
		Avatar:   avatar,
		Token:    info.Token,
	}, nil
}

// joinFileURL 对应 FileService::format($domain, $uri)。
//
// 原实现：
//
//	if ('/' == substr($domain, -1)) { $domain = substr_replace($domain,'',-1,1); }  // 去尾 /
//	if ('/' == substr($uri, 0, 1))  { $uri    = substr_replace($uri,'',0,1); }      // 去首 /
//	return trim($domain) . '/' . trim($uri);
//
// ⚠️ 但 getFileUrl 前面还有两条短路：
//
//	if (strstr($uri,'http://'))  return $uri;
//	if (strstr($uri,'https://')) return $uri;
//
// 也就是**已是完整 URL 的原样返回**。缺了这两条会给外部头像 URL 拼上前缀。
func joinFileURL(domain, uri string) string {
	if strings.Contains(uri, "http://") || strings.Contains(uri, "https://") {
		return uri
	}
	// ⚠️ 只去掉**一个** '/'，不是全部。原实现用的是
	//   substr_replace($domain,'',$domainLen-1,1)  /  substr_replace($uri,'',0,1)
	// 用 strings.TrimRight/TrimLeft 会去掉全部连续斜杠，对 "https://x.cn//"
	// 这种输入结果不同（PHP 会留下一个 '/'）。
	if len(domain) > 0 && domain[len(domain)-1] == '/' {
		domain = domain[:len(domain)-1]
	}
	if len(uri) > 0 && uri[0] == '/' {
		uri = uri[1:]
	}
	return strings.TrimSpace(domain) + "/" + strings.TrimSpace(uri)
}
