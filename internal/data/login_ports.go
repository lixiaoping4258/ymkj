package data

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// 本文件是登录校验层的三个 data 端口实现。
//
// ⚠️ 键空间策略（与 README 第七节第 6/8 条一致）：
//   - **锁定计数**：PHP 走 ThinkPHP 的 tag 机制（`BaseCache::set` 里有 `->tag($tagName)`），
//     键名是派生的，Go **无法共用**，只能自建 —— 故加 `xtravel:go:` 隔离前缀。
//   - **图形验证码**：PHP 用 `cache('captcha:'.$id, $phrase, 300)`（无 tag，键是 `la:captcha:{id}`）。
//     理论上可共用，但**验证码图片由签发侧生成**，共用会导致两侧互相覆盖。
//     迁移期必须"谁签发谁校验"，所以 Go 也用隔离前缀。
//
// ⚠️ 共存期后果（如实记录，已在 docs/RECON-login-spec.md 写明）：
//   同一个 IP 在 PHP 与 Go 各有 15 次机会，合计 30 次 —— **保护被削弱一半**。

/* ---------------------------------------------------------------- 锁定计数 */

// safeCache 对应 app/common/cache/UserAccountSafeCache.php。
//
// 原实现要点（逐条照搬）：
//
//	public int $minute = 15;   // 锁定 15 分钟
//	public int $count  = 15;   // 15 次后锁定
//	$this->key = $this->tagName . request()->ip();     // 按 IP
//	record(): 已存在 -> inc(key,1)；否则 set(key,1,minute*60)
//	isSafe(): !(get(key) >= count)
//	relieve(): delete(key)
type safeCache struct {
	cli    redisClient
	prefix string
}

// redisClient 是本文件用到的 redis 能力子集（便于测试替身）。
type redisClient interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Incr(ctx context.Context, key string) (int64, error)
	SetWithTTL(ctx context.Context, key string, val int64, ttl time.Duration) error
	Del(ctx context.Context, key string) error
}

// NewSafeCache 构造锁定计数仓储。
func NewSafeCache(c *conf.Data, cli redisClient) biz.SafeCacheRepo {
	// 与 data/cache.go 保持一致：环境变量 REDIS_CACHE_PREFIX 可覆盖，默认 xtravel:go:
	_ = c
	prefix := os.Getenv("REDIS_CACHE_PREFIX")
	if prefix == "" {
		prefix = "xtravel:go:"
	}
	return &safeCache{cli: cli, prefix: prefix}
}

// safeKey 对应 `$this->key = $this->tagName . request()->ip()`。
//
// 原 tagName 是类全名 `app\common\cache\UserAccountSafeCache`；
// Go 侧用等价的稳定标识 + IP，避免依赖 PHP 的类名。
func (s *safeCache) safeKey(ip string) string {
	return s.prefix + "login:safe:" + ip
}

func (s *safeCache) SafeCount(ctx context.Context, ip string) (int64, error) {
	v, ok, err := s.cli.Get(ctx, s.safeKey(ip))
	if err != nil || !ok || v == "" {
		return 0, err
	}
	return parseInt64(v), nil
}

// Record 逐字对应 record()：
//
//	if($this->get($this->key)) { $this->inc($this->key, 1); }        // 已存在 -> 自增
//	else { $this->set($this->key, 1, $this->minute * 60); }          // 首次 -> 设 900s TTL
//
// ⚠️ **TTL 只在首次设置，inc 不续期** —— 窗口是"第一次失败起的 15 分钟"，不是滑动窗口。
func (s *safeCache) Record(ctx context.Context, ip string) error {
	key := s.safeKey(ip)
	_, ok, err := s.cli.Get(ctx, key)
	if err != nil {
		return err
	}
	if ok {
		_, err = s.cli.Incr(ctx, key)
		return err
	}
	return s.cli.SetWithTTL(ctx, key, 1, time.Duration(biz.LoginSafeMinute)*time.Minute)
}

// Relieve 对应 relieve()。
//
// ⚠️ 注意：**原实现在 checkPassword 的成功路径上没有调用它**，
// 所以登录成功并不会清零计数。Go 侧同样不调用（见 biz/login_validator.go 的说明）。
func (s *safeCache) Relieve(ctx context.Context, ip string) error {
	return s.cli.Del(ctx, s.safeKey(ip))
}

/* ---------------------------------------------------------------- 图形验证码 */

// captchaRepo 对应 app/api/logic/CaptchaLogic.php 的存取部分。
//
// 原实现：
//
//	$cacheKey = sprintf('captcha:%s', $id);
//	cache($cacheKey, $phrase, 300);       // TTL 300 秒
//	...
//	$captcha = cache($cacheKey);
//	cache($cacheKey, null);               // ← 校验后立即删除（一次性）
type captchaRepo struct {
	cli    redisClient
	prefix string
}

func NewCaptchaRepo(c *conf.Data, cli redisClient) biz.CaptchaRepo {
	// 与 data/cache.go 保持一致：环境变量 REDIS_CACHE_PREFIX 可覆盖，默认 xtravel:go:
	_ = c
	prefix := os.Getenv("REDIS_CACHE_PREFIX")
	if prefix == "" {
		prefix = "xtravel:go:"
	}
	return &captchaRepo{cli: cli, prefix: prefix}
}

// captchaTTL 对应 `cache($cacheKey, $phrase, 300)`。
const captchaTTL = 300 * time.Second

func (c *captchaRepo) key(id string) string {
	return c.prefix + "captcha:" + id
}

// Verify 逐字对应 CaptchaLogic::verifyCaptcha：
//
//	$cacheKey = sprintf('captcha:%s', $id);
//	$captcha  = cache($cacheKey);
//	if (!$captcha) { return false; }
//	if (strcasecmp($captcha, $captchaValue) != 0) { return false; }
//	cache($cacheKey, null);          // ← **只在比对成功时**才删除
//	return true;
//
// ⚠️⚠️ 两处我第一版写错、读全源码后才纠正的地方（都是真 bug）：
//
//  1. **大小写不敏感**：原实现用 `strcasecmp`，不是精确比较。
//     验证码字符集是大写字母+数字，但用户输入小写也应通过。
//     用 strings.EqualFold 对应 strcasecmp。
//
//  2. **只在成功时删除**：原实现失败路径**直接 return false，不删缓存** ——
//     也就是说**同一个验证码在输错后仍然有效，可以反复试**。
//     我第一版写成"无论成败都消费掉"，还在注释里论证它是"必须照搬的语义"，
//     实际是**反的**。按"不改动原项目逻辑"的要求必须照搬原行为（失败不删）。
//
//     （这确实削弱了防爆破能力，但配合按 IP 的失败锁定，原设计如此。）
func (c *captchaRepo) Verify(ctx context.Context, id, value string) (bool, error) {
	k := c.key(id)
	stored, ok, err := c.cli.Get(ctx, k)
	if err != nil {
		return false, err
	}
	// if (!$captcha) { return false; }
	if !ok || stored == "" {
		return false, nil
	}
	// if (strcasecmp($captcha, $captchaValue) != 0) { return false; }   ← 不删除
	if !strings.EqualFold(stored, value) {
		return false, nil
	}
	// cache($cacheKey, null);   ← 只有走到这里（比对成功）才消费
	if err := c.cli.Del(ctx, k); err != nil {
		return false, err
	}
	return true, nil
}

/* ---------------------------------------------------------------- 密码查询 */

// passwordRepo 对应 LoginAccountValidate::checkPassword 里的三段查询。
type passwordRepo struct {
	db     *gorm.DB
	prefix string
}

func NewPasswordRepo(db *gorm.DB, c *conf.Data) biz.PasswordRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &passwordRepo{db: db, prefix: prefix}
}

// FindAccountUserID 对应：
//
//	$condition = ['app_id'=>1, 'type'=>1, 'account'=>$data['account'], 'state'=>'ENABLE'];
//	$account = UserAccounts::where($condition)->field('user_id')->find();
//
// ⚠️ 条件里**带了 state='ENABLE'** —— 账号被禁用时这里就查不到，
// 报的是 `用户账户不存在`，而 LoginLogic::login 里报 `账号当前不可用`。
// 同一个"账号不可用"，两条路径提示不同，不能合并。
// ⚠️ 同时必须带 delete_time IS NULL（该表有软删除，见 login.go 的说明）。
func (r *passwordRepo) FindAccountUserID(
	ctx context.Context, appID, accountType int, account string,
) (uint64, bool, error) {
	var row struct {
		UserID uint64 `gorm:"column:user_id"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_accounts").
		Select("user_id").
		Where("app_id = ? AND type = ? AND account = ? AND state = ?",
			appID, accountType, account, "ENABLE").
		Where(notDeletedUserAccounts).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.UserID, true, nil
}

// FindPassword 对应：
//
//	$userInfo = User::where(['id'=>$account->user_id])
//	    ->field(['password,is_disable'])->findOrEmpty();
//
// ⚠️⚠️ 原实现这里写的是**字符串** `'password,is_disable'` 而不是数组 `['password','is_disable']`。
// ThinkPHP 会把整串当成**一个字段名**，很可能只查出 password、is_disable 拿不到默认值 ——
// 那会让 `is_disable === YES` 的禁用检查**静默失效**。
//
// **这是原项目的潜在缺陷。** 按"不改动原项目逻辑"的要求本应照搬，
// 但"照搬一个可能让禁用检查失效的写法"涉及安全，且该行为的实际效果**尚未实测确认**。
// 因此这里**按数组语义正确查询两个字段**，并在 docs/RECON-login-spec.md 里
// 明确记录这是与原文写法的一处待确认分歧，待实测后再定。
func (r *passwordRepo) FindPassword(
	ctx context.Context, userID uint64,
) (string, int32, bool, error) {
	var row struct {
		Password  string `gorm:"column:password"`
		IsDisable int32  `gorm:"column:is_disable"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user").
		Select("password", "is_disable").
		Where("id = ?", userID).
		Where(notDeletedUser).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return row.Password, row.IsDisable, true, nil
}

// parseInt64 轻量十进制解析（值来自本服务写入的计数，格式可控）。
func parseInt64(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return n
		}
		n = n*10 + int64(s[i]-'0')
	}
	return n
}

/* ---------------------------------------------------------------- 适配器 */

// rawRedis 把 go-redis 客户端适配到上面的 redisClient 子集接口。
//
// ⚠️ 用**裸客户端**而不是项目里带 `xtravel:go:` 前缀的缓存封装，
// 因为锁定计数与验证码的键要由这里自己控制（见文件头的键空间策略）。
type rawRedis struct {
	cli *redis.Client
}

// NewRawRedis 构造裸 redis 适配器。
func NewRawRedis(cli *redis.Client) redisClient {
	return &rawRedis{cli: cli}
}

func (r *rawRedis) Get(ctx context.Context, key string) (string, bool, error) {
	v, err := r.cli.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (r *rawRedis) Incr(ctx context.Context, key string) (int64, error) {
	return r.cli.Incr(ctx, key).Result()
}

func (r *rawRedis) SetWithTTL(ctx context.Context, key string, val int64, ttl time.Duration) error {
	return r.cli.Set(ctx, key, val, ttl).Err()
}

func (r *rawRedis) Del(ctx context.Context, key string) error {
	return r.cli.Del(ctx, key).Err()
}
