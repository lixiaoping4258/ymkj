package biz

import (
	"context"
	"errors"

	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// 本文件对应原项目 app/api/validate/LoginAccountValidate.php。
//
// ⚠️⚠️ 分层必须与原实现一致 —— 这是登录链路里最容易做错的地方：
//
//	LoginController::account()
//	  └─ (new LoginAccountValidate())->post()->goCheck()
//	       ├─ 规则：terminal / scene / account 的 require|in
//	       └─ checkConfig($scene)                       ← 逐 scene 分支
//	            ├─ ConfigService::get('login','login_way') 白名单
//	            ├─ scene=2: checkCode()               手机验证码
//	            └─ scene=1: 图形验证码 -> checkPassword()
//	                 └─ 锁定检查 -> 查账号 -> 查用户 -> 比对密码 -> 记失败
//	  └─ LoginLogic::login($params)                     ← 已在 login.go 实现
//
// **密码校验不在 login() 里**。把它塞进 login() 会让错误信息的产生位置与顺序改变。

// 逐字对照 LoginAccountValidate::$message
var (
	ErrTerminalRequired = errors.New("终端参数缺失")
	ErrTerminalInvalid  = errors.New("终端参数状态值不正确")
	ErrSceneRequired    = errors.New("场景不能为空")
	ErrSceneInvalid     = errors.New("场景值错误")
	ErrAccountRequired  = errors.New("请输入账号")
	ErrPasswordRequired = errors.New("请输入密码")
	ErrCaptchaIDInvalid = errors.New("图形验证码ID不正确")
	ErrCaptchaWrong     = errors.New("图形验证码不正确")
	ErrLoginWayInvalid  = errors.New("不支持的登录方式")
	ErrCodeRequired     = errors.New("请输入手机验证码")
	// checkCode 里 `return '验证码错误';`
	ErrSmsCodeWrong = errors.New("验证码错误")
	// checkPassword 里的 5 条
	ErrAccountNotExist = errors.New("用户账户不存在")
	ErrUserNotExist    = errors.New("用户不存在")
	ErrUserDisabled    = errors.New("用户已禁用")
	ErrPasswordWrong   = errors.New("密码错误")
)

// LoginTerminals 对应 UserTerminalEnum 的 6 个合法取值：
// 1 微信小程序 / 2 微信公众号 / 3 手机H5 / 4 电脑PC / 5 苹果app / 6 安卓app
var LoginTerminals = []int32{1, 2, 3, 4, 5, 6}

// SafeCacheRepo 对应 UserAccountSafeCache —— **按 IP** 的登录失败计数。
//
// 原实现（app/common/cache/UserAccountSafeCache.php，81 行）：
//
//	public int $minute = 15;    // 锁定 15 分钟
//	public int $count  = 15;    // 15 次后锁定
//	$this->key = $this->tagName . request()->ip();
//	record(): 已存在 -> inc(key,1)；否则 set(key,1,minute*60)
//	isSafe(): !(get(key) >= count)
//
// ⚠️ 两个必须照搬的细节：
//  1. **按 IP 计数，不是按账号**
//  2. **TTL 只在首次失败时设置**，`inc` 不续期 —— 窗口是"第一次失败起的 15 分钟"，不是滑动窗口
//
// ⚠️ 共存期限制：PHP 侧走 ThinkPHP 的 tag 机制（`BaseCache::set` 里有 `->tag()`），
// Go 无法共用那一把键，只能自建。结果是**同一个 IP 在共存期共有 30 次机会（两侧各 15）**，
// 保护被削弱一半。这是固有限制，已在 docs/RECON-login-spec.md 记录。
type SafeCacheRepo interface {
	// SafeCount 返回当前 IP 的失败次数（不存在返回 0）
	SafeCount(ctx context.Context, ip string) (int64, error)
	// Record 记一次失败
	Record(ctx context.Context, ip string) error
	// Relieve 清除该 IP 的计数（登录成功时调用）
	Relieve(ctx context.Context, ip string) error
}

// LoginSafeCount / LoginSafeMinute 逐字对应 UserAccountSafeCache 的两个常量。
const (
	LoginSafeCount  = 15
	LoginSafeMinute = 15
)

// CaptchaRepo 对应 CaptchaLogic 的验证码存取。
//
// 原实现（app/api/logic/CaptchaLogic.php）：
//
//	$cacheKey = sprintf('captcha:%s', $id);
//	cache($cacheKey, $phrase, 300);      // 生成时，TTL 300 秒
//	...
//	$captcha = cache($cacheKey);
//	cache($cacheKey, null);              // 校验后**立即删除**（一次性）
type CaptchaRepo interface {
	// Set 对应 `cache('captcha:'.$id, $phrase, 300)` —— TTL 300 秒。
	Set(ctx context.Context, id, phrase string) error
	// Verify 对应 CaptchaLogic::verifyCaptcha。
	// 语义见 data/login_ports.go 的实现说明（大小写不敏感、只在成功时消费）。
	Verify(ctx context.Context, id, value string) (bool, error)
}

// PasswordRepo 对应 checkPassword 里的三段查询。
type PasswordRepo interface {
	// FindAccountUserID 对应：
	//   UserAccounts::where(['app_id'=>1,'type'=>1,'account'=>..., 'state'=>'ENABLE'])
	//                ->field('user_id')->find()
	//
	// ⚠️ 注意查询条件里**带了 state='ENABLE'**。所以账号被禁用时这里查不到，
	// 报的是 `用户账户不存在`，而 LoginLogic::login 里报的是 `账号当前不可用`。
	// 同一个"账号不可用"，两条路径的提示不同 —— 必须按顺序复刻，不能合并。
	FindAccountUserID(ctx context.Context, appID, accountType int, account string) (uint64, bool, error)
	// FindPassword 对应 User::where(['id'=>...])->field(['password,is_disable'])->findOrEmpty()
	// 返回 (passwordHash, isDisable, found, error)
	FindPassword(ctx context.Context, userID uint64) (string, int32, bool, error)
}

// LoginValidator 登录校验器。
type LoginValidator struct {
	password PasswordRepo
	captcha  CaptchaRepo
	safe     SafeCacheRepo
	// salt 对应 Config::get('project.unique_identification')，
	// 实测 .env 覆盖为 "aaaa123"（7 位）。
	// 从 *conf.Auth 读取（wire 无法注入裸 string）。
	salt string
	// sms 用于 scene=2（手机验证码）的校验
	sms *SmsUsecase
}

// NewLoginValidator 构造登录校验器。
// salt 取自 conf.Auth.UniqueIdentification，与 data.NewLoginRepo 用的是同一个来源。
func NewLoginValidator(p PasswordRepo, c CaptchaRepo, s SafeCacheRepo, auth *conf.Auth, sms *SmsUsecase) *LoginValidator {
	salt := ""
	if auth != nil {
		salt = auth.UniqueIdentification
	}
	return &LoginValidator{password: p, captcha: c, safe: s, salt: salt, sms: sms}
}

// LoginParams 是 POST /v1/login/account 的入参。
type LoginParams struct {
	Terminal  int32
	Scene     int32
	Account   string
	Password  string
	CaptchaID string
	Captcha   string
	Code      string
}

// Validate 逐字对应 LoginAccountValidate 的规则段（$rule + $message）。
//
//	'terminal' => 'require|in:1,2,3,4,5,6'
//	'scene'    => 'require|in:1,2|checkConfig'
//	'account'  => 'require'
//
// ⚠️ 校验顺序按 $rule 的声明顺序：terminal -> scene -> account。
// 顺序错了，同一个错误请求会返回不同的提示。
func (v *LoginValidator) Validate(p *LoginParams) error {
	// terminal
	if p.Terminal == 0 {
		return ErrTerminalRequired
	}
	if !containsInt32(LoginTerminals, p.Terminal) {
		return ErrTerminalInvalid
	}
	// scene
	if p.Scene == 0 {
		return ErrSceneRequired
	}
	// ⚠️ login_way 实际是 ['1','2']（字符串），PHP 的 in_array 默认**宽松比较**，
	// 所以 in_array(1, ['1','2']) 为真。这里直接按整数判断，结果一致。
	// **不要**去读配置再跟字符串比 —— 那样会因类型不匹配而全部拒绝。
	if p.Scene != int32(LoginSceneAccountPassword) && p.Scene != int32(LoginSceneMobileCaptcha) {
		return ErrSceneInvalid
	}
	// account
	if p.Account == "" {
		return ErrAccountRequired
	}
	return nil
}

// CheckConfig 对应 checkConfig($scene) —— 逐 scene 的深度校验。
//
// 必须在 Validate 通过之后调用（顺序与 PHP 一致）。
func (v *LoginValidator) CheckConfig(ctx context.Context, p *LoginParams, clientIP string) error {
	switch p.Scene {
	case int32(LoginSceneMobileCaptcha):
		if p.Code == "" {
			return ErrCodeRequired
		}
		// 对应 checkCode($data['code'], [], $data)：
		//
		//	$smsDriver = new SmsDriver();
		//	$result = $smsDriver->verify($data['account'], $code, NoticeEnum::LOGIN_CAPTCHA);
		//	if($result) { return true; }
		//	return '验证码错误';
		//
		// ⚠️ 注意 verify 的第一个参数是 **$data['account']（即手机号）**，
		//    不是单独的 mobile 字段。
		// ⚠️ sceneId 传 NoticeEnum::LOGIN_CAPTCHA = 101。
		ok, err := v.sms.Verify(ctx, p.Account, p.Code, NoticeLoginCaptcha)
		if err != nil {
			return err
		}
		if !ok {
			return ErrSmsCodeWrong
		}
		return nil

	case int32(LoginSceneAccountPassword):
		// ① 密码存在性
		if p.Password == "" {
			return ErrPasswordRequired
		}
		// ② 图形验证码（在密码校验**之前**）
		if p.CaptchaID == "" {
			return ErrCaptchaIDInvalid
		}
		ok, err := v.captcha.Verify(ctx, p.CaptchaID, p.Captcha)
		if err != nil {
			return err
		}
		if !ok {
			return ErrCaptchaWrong
		}
		// ③ 密码比对（含锁定）
		return v.CheckPassword(ctx, p, clientIP)

	default:
		return ErrLoginWayInvalid
	}
}

// CheckPassword 逐字对应 LoginAccountValidate::checkPassword。
//
// 原实现（L125）：
//
//	$userAccountSafeCache = new UserAccountSafeCache();
//	if (!$userAccountSafeCache->isSafe()) {
//	    return '密码连续' . $count . '次输入错误，请' . $minute . '分钟后重试';
//	}
//	$account = UserAccounts::where([... 'state'=>'ENABLE'])->field('user_id')->find();
//	if (!$account) { return '用户账户不存在'; }
//	$userInfo = User::where(['id'=>$account->user_id])->field(['password,is_disable'])->findOrEmpty();
//	if ($userInfo->isEmpty()) { return '用户不存在'; }
//	if ($userInfo['is_disable'] === YesNoEnum::YES) { return '用户已禁用'; }
//	if (empty($userInfo['password'])) { $safe->record(); return '用户不存在'; }
//	if ($userInfo['password'] !== create_password($password, $salt)) {
//	    $safe->record(); return '密码错误';
//	}
//
// ⚠️ 注意原实现在**成功路径上没有调用 relieve()**！只有失败时才 record()。
// 所以计数不会因为登录成功而清零，只能等 15 分钟 TTL 自然过期。
// 这是原实现的行为，按"不改动原逻辑"照搬 —— 见下方说明。
func (v *LoginValidator) CheckPassword(ctx context.Context, p *LoginParams, clientIP string) error {
	const appID = 1
	const accountType = AccountTypeAccount // type = 1

	// ① 锁定检查
	n, err := v.safe.SafeCount(ctx, clientIP)
	if err != nil {
		return err
	}
	if n >= LoginSafeCount {
		// '密码连续' . $count . '次输入错误，请' . $minute . '分钟后重试'
		return &LockedError{Count: n, Minute: LoginSafeMinute}
	}

	// ② 查账号（条件里带 state='ENABLE'）
	userID, found, err := v.password.FindAccountUserID(ctx, appID, accountType, p.Account)
	if err != nil {
		return err
	}
	if !found {
		return ErrAccountNotExist
	}

	// ③ 查用户
	hash, isDisable, found, err := v.password.FindPassword(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return ErrUserNotExist
	}
	if isDisable == 1 { // YesNoEnum::YES
		return ErrUserDisabled
	}
	if hash == "" { // empty($userInfo['password'])
		if err := v.safe.Record(ctx, clientIP); err != nil {
			return err
		}
		return ErrUserNotExist
	}

	// ④ 比对密码
	// ⚠️ login.go 与这里都要拿到盐；Validator 持有 CreatePassword 所需的 salt
	// 由调用方通过 WithSalt 注入（见 NewLoginValidator 的说明）。
	if hash != CreatePassword(p.Password, v.salt) {
		if err := v.safe.Record(ctx, clientIP); err != nil {
			return err
		}
		return ErrPasswordWrong
	}

	// ⚠️ 原实现在这里**没有** $safe->relieve()。
	// 也就是说登录成功后失败计数不会被清除，只能等 15 分钟过期。
	// 按"不改动原项目逻辑"的要求照搬此行为，**不额外调用 Relieve**。
	// （Relieve 仍保留在端口上，因为原类有这个方法，且未来可能被别处调用。）
	return nil
}

// LockedError 对应 '密码连续' . $count . '次输入错误，请' . $minute . '分钟后重试'
//
// ⚠️ 消息里的两个数字都来自运行时的值（$userAccountSafeCache->count / ->minute），
// 其中 count 是**当前已错误次数**、minute 是**固定 15**。
// 拼串方式必须逐字一致。
type LockedError struct {
	Count  int64
	Minute int
}

func (e *LockedError) Error() string {
	return "密码连续" + itoa(e.Count) + "次输入错误，请" + itoa(int64(e.Minute)) + "分钟后重试"
}

// containsInt32 判断切片中是否含某值（对应 PHP 的 in_array，此处为精确匹配）。
func containsInt32(xs []int32, v int32) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// itoa 轻量整数转字符串（避免为两处拼接引入 strconv）。
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [24]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
