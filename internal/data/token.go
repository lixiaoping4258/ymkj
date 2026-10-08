package data

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

/* ---------------------------------------------------------------- session 仓储 */

type userSessionRepo struct {
	db *gorm.DB
}

func NewUserSessionRepo(db *gorm.DB) biz.UserSessionRepo {
	return &userSessionRepo{db: db}
}

// FindByToken 对应 UserSession::where('token','=',$token)->findOrEmpty()。
//
// 注意**不带**过期条件：原项目只有 UserTokenCache::setUserInfo 里那一处
// 才加 `expire_time > time()`，overtimeToken / expireToken 都是只按 token 查。
// 过期判断放在 biz 层做，保持和原实现一致。
func (r *userSessionRepo) FindByToken(ctx context.Context, token string) (*biz.UserSession, error) {
	var row model.UserSession
	err := r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("token = ?", token).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toBizSession(&row), nil
}

// FindByUserTerminal 对应
// UserSession::where([['user_id','=',$userId],['terminal','=',$terminal]])->findOrEmpty()。
func (r *userSessionRepo) FindByUserTerminal(ctx context.Context, userID uint64, terminal int32) (*biz.UserSession, error) {
	var row model.UserSession
	err := r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("user_id = ? AND terminal = ?", userID, terminal).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toBizSession(&row), nil
}

func (r *userSessionRepo) Create(ctx context.Context, s *biz.UserSession) error {
	row := model.UserSession{
		UserID:     s.UserID,
		Terminal:   s.Terminal,
		Token:      s.Token,
		ExpireTime: s.ExpireTime,
		UpdateTime: s.UpdateTime,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	s.ID = row.ID
	return nil
}

// Update 只更新 token/expire_time/update_time 三列。
//
// 不用 GORM 的 Save：那张表是 likeadmin 的老表，整行保存会把没读出来的列
// 覆盖成零值。原代码也遇到过类似的坑（expireToken 里特意用 update 而不是 save）。
func (r *userSessionRepo) Update(ctx context.Context, s *biz.UserSession) error {
	return r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("id = ?", s.ID).
		Updates(map[string]any{
			"token":       s.Token,
			"expire_time": s.ExpireTime,
			"update_time": s.UpdateTime,
		}).Error
}

// ExpireOnly 对应 UserTokenService::expireToken 里那段带注释的 update：
// 「使用 update 方法避免触发完整保存，避免 UserSession 保存时缺少 user_id」。
func (r *userSessionRepo) ExpireOnly(ctx context.Context, id uint64, expireTime, updateTime int64) error {
	return r.db.WithContext(ctx).
		Model(&model.UserSession{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"expire_time": expireTime,
			"update_time": updateTime,
		}).Error
}

func toBizSession(row *model.UserSession) *biz.UserSession {
	return &biz.UserSession{
		ID:         row.ID,
		UserID:     row.UserID,
		Terminal:   row.Terminal,
		Token:      row.Token,
		UpdateTime: row.UpdateTime,
		ExpireTime: row.ExpireTime,
	}
}

/* ---------------------------------------------------------------- token 缓存 */

// userTokenCache 对应 app/common/cache/UserTokenCache.php。
//
// ⚠️ 键前缀必须与 ThinkPHP 隔离。原实现走 BaseCache，而 BaseCache::set 会
// 调 store()->tag($tagName)->set(...) —— 存进 Redis 的是 ThinkPHP 的
// tag 结构 + 序列化格式，Go 直接用会读出乱码（这也是之前那个
// "only array cache can be push" 事故的同一套机制）。
// 所以这里统一用 xtravel:go:token_user_{token}，互不干扰。
type userTokenCache struct {
	cli    *redis.Client
	keyPre string
}

func NewUserTokenCache(cli *redis.Client, cfg biz.AuthConfig) biz.UserTokenCache {
	pre := cfg.TokenCachePrefix
	if pre == "" {
		pre = "xtravel:go:"
	}
	return &userTokenCache{cli: cli, keyPre: pre + "token_user_"}
}

func (c *userTokenCache) k(token string) string { return c.keyPre + token }

func (c *userTokenCache) Get(ctx context.Context, token string) (*biz.UserInfo, error) {
	b, err := c.cli.Get(ctx, c.k(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var info biz.UserInfo
	if err := json.Unmarshal(b, &info); err != nil {
		// 脏数据当未命中，顺手删掉
		_ = c.cli.Del(ctx, c.k(token)).Err()
		return nil, nil
	}
	// 双保险：缓存里存的过期时间可能已经过了（TTL 由 Redis 兜底，这里再判一次）
	if info.ExpireTime <= time.Now().Unix() {
		_ = c.cli.Del(ctx, c.k(token)).Err()
		return nil, nil
	}
	return &info, nil
}

func (c *userTokenCache) Set(ctx context.Context, token string, info *biz.UserInfo, ttl time.Duration) error {
	if ttl <= 0 {
		return nil
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return c.cli.Set(ctx, c.k(token), b, ttl).Err()
}

func (c *userTokenCache) Del(ctx context.Context, token string) error {
	return c.cli.Del(ctx, c.k(token)).Err()
}

/* ---------------------------------------------------------------- 配置装配 */

// NewAuthConfig 把 conf.Auth 转成 biz.AuthConfig。
//
// 默认值取自原项目 config/project.php 的 user_token 段，
// 配置缺失时也不会退化成 0（否则 token 一签发就过期）。
func NewAuthConfig(c *conf.Auth, logger log.Logger) biz.AuthConfig {
	l := log.NewHelper(logger)
	cfg := biz.AuthConfig{
		ExpireDuration:   7 * 24 * time.Hour, // 3600*24*7
		BeExpireDuration: time.Hour,          // 3600
		UniqueIdent:      "likeadmin",        // 原代码的默认盐
		TokenCachePrefix: "xtravel:go:",
	}
	if c == nil {
		return cfg
	}
	if c.ExpireDuration > 0 {
		cfg.ExpireDuration = time.Duration(c.ExpireDuration) * time.Second
	}
	if c.BeExpireDuration > 0 {
		cfg.BeExpireDuration = time.Duration(c.BeExpireDuration) * time.Second
	}
	if c.UniqueIdentification != "" {
		cfg.UniqueIdent = c.UniqueIdentification
	}
	if c.TokenCachePrefix != "" {
		cfg.TokenCachePrefix = c.TokenCachePrefix
	}
	l.Infof("鉴权配置: expire=%s beExpire=%s 盐长度=%d",
		cfg.ExpireDuration, cfg.BeExpireDuration, len(cfg.UniqueIdent))
	return cfg
}
