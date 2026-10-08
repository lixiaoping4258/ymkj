package biz

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目三个东西：
//   app/api/http/middleware/LoginMiddleware.php   （鉴权流程）
//   app/api/service/UserTokenService.php          （签发/续期/失效）
//   app/common/cache/UserTokenCache.php           （token -> 用户信息缓存）

// UserInfo 是 token 缓存里存的用户信息。
//
// 字段名（json tag）必须和原项目 UserTokenCache::setUserInfo 里的数组键**逐字一致** ——
// 它是直接 json 存进缓存的，键名变了缓存就不兼容。
type UserInfo struct {
	UserID     uint64 `json:"user_id"`
	Nickname   string `json:"nickname"`
	Token      string `json:"token"`
	Sn         uint64 `json:"sn"`
	Mobile     string `json:"mobile"`
	Avatar     string `json:"avatar"`
	Terminal   int32  `json:"terminal"`
	ExpireTime int64  `json:"expire_time"`
}

// UserSession 对应表 x_user_session 的一行。
//
// 注意 expire_time / update_time 是 **int 时间戳**，不是 datetime
// （ThinkPHP 的 auto_timestamp 对这张表写的是 int）。
type UserSession struct {
	ID         uint64
	UserID     uint64
	Terminal   int32
	Token      string
	UpdateTime int64
	ExpireTime int64
}

// UserBrief 是签发/组装 token 缓存时需要的用户字段。
type UserBrief struct {
	ID       uint64
	Nickname string
	Sn       uint64
	Mobile   string
	Avatar   string
}

// UserSessionRepo 对应 UserSession 模型的查询。
type UserSessionRepo interface {
	FindByToken(ctx context.Context, token string) (*UserSession, error)
	FindByUserTerminal(ctx context.Context, userID uint64, terminal int32) (*UserSession, error)
	Create(ctx context.Context, s *UserSession) error
	Update(ctx context.Context, s *UserSession) error
	// ExpireOnly 只更新 expire_time/update_time。
	// 原代码特意用 update() 而不是 save()，避免触发整行保存时缺 user_id 报错。
	ExpireOnly(ctx context.Context, id uint64, expireTime, updateTime int64) error
}

// UserBriefRepo 取用户基础信息。
type UserBriefRepo interface {
	FindBriefByID(ctx context.Context, id uint64) (*UserBrief, error)
}

// UserTokenCache 是 token -> UserInfo 的缓存端口。
type UserTokenCache interface {
	Get(ctx context.Context, token string) (*UserInfo, error)
	Set(ctx context.Context, token string, info *UserInfo, ttl time.Duration) error
	Del(ctx context.Context, token string) error
}

// AuthConfig 对应 config/project.php 的 user_token 段 + .env 的 UNIQUE_IDENTIFICATION。
type AuthConfig struct {
	ExpireDuration   time.Duration // 原: expire_duration    = 3600*24*7
	BeExpireDuration time.Duration // 原: be_expire_duration = 3600
	UniqueIdent      string        // 原: env('project.unique_identification')
	// TokenCachePrefix 是 token 缓存的键前缀。
	// 必须与 ThinkPHP 隔离：原实现走 BaseCache 的 tag 机制，
	// 存的是 TP 自己的序列化格式，共用键会读出乱码。
	TokenCachePrefix string
}

// UserTokenUsecase 是鉴权与 token 生命周期的唯一入口。
type UserTokenUsecase struct {
	sessions UserSessionRepo
	users    UserBriefRepo
	cache    UserTokenCache
	cfg      AuthConfig
	log      *log.Helper
}

func NewUserTokenUsecase(
	sessions UserSessionRepo,
	users UserBriefRepo,
	cache UserTokenCache,
	cfg AuthConfig,
	logger log.Logger,
) *UserTokenUsecase {
	return &UserTokenUsecase{
		sessions: sessions,
		users:    users,
		cache:    cache,
		cfg:      cfg,
		log:      log.NewHelper(logger),
	}
}

// GetUserInfo 对应 UserTokenCache::getUserInfo：缓存优先，未命中回源。
//
// 返回 (nil, nil) 表示这个 token 无效或已过期 —— 对应原项目返回 false 的分支。
func (uc *UserTokenUsecase) GetUserInfo(ctx context.Context, token string) (*UserInfo, error) {
	if token == "" {
		return nil, nil
	}
	info, err := uc.cache.Get(ctx, token)
	if err != nil {
		// 缓存故障不能把用户挡在门外，降级查库
		uc.log.WithContext(ctx).Warnf("读取 token 缓存失败: %v", err)
	} else if info != nil {
		return info, nil
	}
	return uc.SetUserInfo(ctx, token)
}

// SetUserInfo 对应 UserTokenCache::setUserInfo：从库里组装并写缓存。
//
// ⚠️ 与原实现的一处**有意偏差**：
// 原代码在用户行不存在时（比如账号已软删除）会让 $user 为 null，
// 然后 `$user->id` 触发 PHP 警告并得到 null，最终拼出一个 user_id=null 的
// 「用户信息」并写入缓存 —— 因为非空数组为真，鉴权**照样通过**。
// 这里改成返回 nil（视为 token 无效），宁可变严格也不要放行幽灵用户。
func (uc *UserTokenUsecase) SetUserInfo(ctx context.Context, token string) (*UserInfo, error) {
	sess, err := uc.sessions.FindByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	// PHP: where token=? AND expire_time > time()
	if sess == nil || sess.ExpireTime <= time.Now().Unix() {
		return nil, nil
	}

	u, err := uc.users.FindBriefByID(ctx, sess.UserID)
	if err != nil {
		return nil, err
	}
	if u == nil {
		uc.log.WithContext(ctx).Warnf("token 有效但用户不存在或已软删除: token=%s user_id=%d",
			maskToken(token), sess.UserID)
		return nil, nil
	}

	info := &UserInfo{
		UserID:     u.ID,
		Nickname:   u.Nickname,
		Token:      token,
		Sn:         u.Sn,
		Mobile:     u.Mobile,
		Avatar:     u.Avatar,
		Terminal:   sess.Terminal,
		ExpireTime: sess.ExpireTime,
	}

	ttl := time.Until(time.Unix(sess.ExpireTime, 0))
	if ttl <= 0 {
		return nil, nil
	}
	if err := uc.cache.Set(ctx, token, info, ttl); err != nil {
		uc.log.WithContext(ctx).Warnf("写入 token 缓存失败: %v", err)
	}
	return info, nil
}

// OvertimeToken 对应 UserTokenService::overtimeToken：延长过期时间并刷新缓存。
func (uc *UserTokenUsecase) OvertimeToken(ctx context.Context, token string) (*UserInfo, error) {
	sess, err := uc.sessions.FindByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil // PHP 返回 false
	}
	now := time.Now().Unix()
	sess.ExpireTime = now + int64(uc.cfg.ExpireDuration.Seconds())
	sess.UpdateTime = now
	if err := uc.sessions.Update(ctx, sess); err != nil {
		return nil, err
	}
	return uc.SetUserInfo(ctx, token)
}

// ExpireToken 对应 UserTokenService::expireToken：立即失效（登出）。
func (uc *UserTokenUsecase) ExpireToken(ctx context.Context, token string) (bool, error) {
	sess, err := uc.sessions.FindByToken(ctx, token)
	if err != nil {
		return false, err
	}
	if sess == nil {
		return false, nil
	}
	now := time.Now().Unix()
	// 只用 update，避免整行 save —— 原代码有注释说明原因
	if err := uc.sessions.ExpireOnly(ctx, sess.ID, now, now); err != nil {
		return false, err
	}
	if err := uc.cache.Del(ctx, token); err != nil {
		return false, err
	}
	return true, nil
}

// SetToken 对应 UserTokenService::setToken：登录时签发（或换发）token。
//
// Stage 2 还没迁登录接口，但这个方法要一起迁 —— 否则 user 域的登出/续期
// 没有对应的签发逻辑，逻辑是残缺的。
func (uc *UserTokenUsecase) SetToken(ctx context.Context, userID uint64, terminal int32) (*UserInfo, error) {
	now := time.Now().Unix()
	sess, err := uc.sessions.FindByUserTerminal(ctx, userID, terminal)
	if err != nil {
		return nil, err
	}

	if sess != nil {
		// 先删旧 token 的缓存，再换发
		if err := uc.cache.Del(ctx, sess.Token); err != nil {
			uc.log.WithContext(ctx).Warnf("删除旧 token 缓存失败: %v", err)
		}
		sess.Token = CreateToken(uc.cfg.UniqueIdent, strconv.FormatUint(userID, 10))
		sess.ExpireTime = now + int64(uc.cfg.ExpireDuration.Seconds())
		sess.UpdateTime = now
		if err := uc.sessions.Update(ctx, sess); err != nil {
			return nil, err
		}
		return uc.SetUserInfo(ctx, sess.Token)
	}

	newSess := &UserSession{
		UserID:     userID,
		Terminal:   terminal,
		Token:      CreateToken(uc.cfg.UniqueIdent, strconv.FormatUint(userID, 10)),
		ExpireTime: now + int64(uc.cfg.ExpireDuration.Seconds()),
		UpdateTime: now,
	}
	if err := uc.sessions.Create(ctx, newSess); err != nil {
		return nil, err
	}
	return uc.SetUserInfo(ctx, newSess.Token)
}

// CreateToken 对应全局函数 create_token()：
//
//	$salt        = env('project.unique_identification', 'likeadmin');
//	$encryptSalt = md5($salt . uniqid());
//	return md5($salt . $extra . time() . $encryptSalt);
//
// 产出是 32 位小写 hex，与 x_user_session.token varchar(32) 对得上。
//
// ⚠️ 与原实现的差异（有意修正）：
// PHP 只用 uniqid()（微秒）做熵。Go 里照搬会在 Windows 上出问题 ——
// time.Now() 的时钟分辨率较粗，同一滴答内连续调用拿到的纳秒部分完全相同，
// 实测 200 次循环就出现重复 token。token 有 UNIQUE 约束，
// 撞了轻则登录失败，重则换发时覆盖到别人的会话。
// 所以这里在时间之外**额外加进程内自增序号 + crypto/rand**，
// 保证单进程内绝对不重复、多实例间也几乎不可能撞。
func CreateToken(salt, extra string) string {
	encryptSalt := md5hex(salt + uniqueEntropy())
	return md5hex(salt + extra + strconv.FormatInt(time.Now().Unix(), 10) + encryptSalt)
}

// tokenSeq 保证同一进程内生成的熵严格递增，不依赖时钟分辨率。
var tokenSeq atomic.Uint64

// uniqueEntropy 生成一次性的熵串。
// 形状参考 PHP uniqid()（8 位秒 + 5 位微秒），但额外拼接随机数与自增序号。
func uniqueEntropy() string {
	now := time.Now()
	var rnd [8]byte
	// crypto/rand 失败时还有自增序号兜底，不会退化成可预测
	_, _ = rand.Read(rnd[:])
	return fmt.Sprintf("%08x%05x%s%08x",
		now.Unix(), now.Nanosecond()/1000, hex.EncodeToString(rnd[:]), tokenSeq.Add(1))
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// maskToken 打日志时遮蔽 token，避免完整凭据进日志。
func maskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:8] + "***"
}

/* ---------------------------------------------------------------- 上下文传递 */

type userInfoCtxKey struct{}

// WithUserInfo 把登录用户信息放进 context，供 service 层取用。
// 对应原项目往 $request->userInfo / $request->userId 上挂值。
func WithUserInfo(ctx context.Context, info *UserInfo) context.Context {
	return context.WithValue(ctx, userInfoCtxKey{}, info)
}

// UserInfoFromContext 取登录用户信息。
func UserInfoFromContext(ctx context.Context) (*UserInfo, bool) {
	v, ok := ctx.Value(userInfoCtxKey{}).(*UserInfo)
	return v, ok && v != nil
}

// UserIDFromContext 取用户 ID，未登录返回 0。
// 对应原项目 BaseApiController 里的 $this->userId。
func UserIDFromContext(ctx context.Context) uint64 {
	if info, ok := UserInfoFromContext(ctx); ok {
		return info.UserID
	}
	return 0
}
