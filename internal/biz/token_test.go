package biz

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 用假实现测 UserTokenUsecase，不碰数据库。
// 重点覆盖三个容易出错的地方：
//   1. 缓存未命中要回源并回填
//   2. 过期会话必须判为无效
//   3. **用户已软删除时不能放行**（这是与原实现的有意偏差，见 token.go 注释）

/* ---------------------------------------------------------------- 假实现 */

type fakeSessionRepo struct {
	byToken map[string]*UserSession
	byPair  map[string]*UserSession
	nextID  uint64
	created int
	updated int
	expired int
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{
		byToken: map[string]*UserSession{},
		byPair:  map[string]*UserSession{},
		nextID:  1,
	}
}

func pairKey(userID uint64, terminal int32) string {
	return string(rune(userID)) + "|" + string(rune(terminal))
}

func (f *fakeSessionRepo) FindByToken(_ context.Context, token string) (*UserSession, error) {
	s, ok := f.byToken[token]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSessionRepo) FindByUserTerminal(_ context.Context, userID uint64, terminal int32) (*UserSession, error) {
	s, ok := f.byPair[pairKey(userID, terminal)]
	if !ok {
		return nil, nil
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSessionRepo) Create(_ context.Context, s *UserSession) error {
	s.ID = f.nextID
	f.nextID++
	f.created++
	cp := *s
	f.byToken[s.Token] = &cp
	f.byPair[pairKey(s.UserID, s.Terminal)] = &cp
	return nil
}

func (f *fakeSessionRepo) Update(_ context.Context, s *UserSession) error {
	f.updated++
	cp := *s
	f.byToken[s.Token] = &cp
	f.byPair[pairKey(s.UserID, s.Terminal)] = &cp
	return nil
}

func (f *fakeSessionRepo) ExpireOnly(_ context.Context, id uint64, expireTime, updateTime int64) error {
	f.expired++
	for _, s := range f.byToken {
		if s.ID == id {
			s.ExpireTime = expireTime
			s.UpdateTime = updateTime
		}
	}
	return nil
}

type fakeUserRepo struct {
	users map[uint64]*UserBrief
}

func (f *fakeUserRepo) FindBriefByID(_ context.Context, id uint64) (*UserBrief, error) {
	u, ok := f.users[id]
	if !ok {
		return nil, nil // 对应「用户不存在或已软删除」
	}
	cp := *u
	return &cp, nil
}

type fakeTokenCache struct {
	m       map[string]*UserInfo
	sets    int
	dels    int
	failGet bool
}

func newFakeTokenCache() *fakeTokenCache {
	return &fakeTokenCache{m: map[string]*UserInfo{}}
}

func (f *fakeTokenCache) Get(_ context.Context, token string) (*UserInfo, error) {
	if f.failGet {
		return nil, errors.New("redis down")
	}
	info, ok := f.m[token]
	if !ok {
		return nil, nil
	}
	cp := *info
	return &cp, nil
}

func (f *fakeTokenCache) Set(_ context.Context, token string, info *UserInfo, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("ttl must be positive")
	}
	f.sets++
	cp := *info
	f.m[token] = &cp
	return nil
}

func (f *fakeTokenCache) Del(_ context.Context, token string) error {
	f.dels++
	delete(f.m, token)
	return nil
}

/* ---------------------------------------------------------------- 夹具 */

func newUsecase(t *testing.T, now int64) (*UserTokenUsecase, *fakeSessionRepo, *fakeUserRepo, *fakeTokenCache) {
	t.Helper()
	sessions := newFakeSessionRepo()
	users := &fakeUserRepo{users: map[uint64]*UserBrief{
		7: {ID: 7, Nickname: "测试用户", Sn: 1001, Mobile: "13800000000", Avatar: "a.png"},
	}}
	cache := newFakeTokenCache()
	uc := NewUserTokenUsecase(sessions, users, cache, AuthConfig{
		ExpireDuration:   7 * 24 * time.Hour,
		BeExpireDuration: time.Hour,
		UniqueIdent:      "test-salt",
		TokenCachePrefix: "test:",
	}, log.NewStdLogger(io.Discard))

	// 固定一个有效会话：user_id=7, 30 分钟后过期
	sessions.byToken["tok-valid"] = &UserSession{
		ID: 1, UserID: 7, Terminal: 3, Token: "tok-valid",
		ExpireTime: now + 1800, UpdateTime: now,
	}
	sessions.byPair[pairKey(7, 3)] = sessions.byToken["tok-valid"]

	// 已过期的会话
	sessions.byToken["tok-expired"] = &UserSession{
		ID: 2, UserID: 7, Terminal: 3, Token: "tok-expired",
		ExpireTime: now - 10, UpdateTime: now - 100,
	}
	return uc, sessions, users, cache
}

/* ---------------------------------------------------------------- 用例 */

func TestGetUserInfo_CacheMissFallsBackToDBAndFills(t *testing.T) {
	now := time.Now().Unix()
	uc, _, _, cache := newUsecase(t, now)

	info, err := uc.GetUserInfo(context.Background(), "tok-valid")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info == nil {
		t.Fatal("有效 token 应当返回用户信息")
	}
	if info.UserID != 7 || info.Nickname != "测试用户" || info.Terminal != 3 {
		t.Fatalf("用户信息不对: %+v", info)
	}
	if cache.sets != 1 {
		t.Fatalf("回源后应当回填缓存一次，实际 %d 次", cache.sets)
	}

	// 第二次应当命中缓存，不再回源
	info2, err := uc.GetUserInfo(context.Background(), "tok-valid")
	if err != nil || info2 == nil {
		t.Fatalf("第二次读取失败: %v %+v", err, info2)
	}
	if cache.sets != 1 {
		t.Fatalf("命中缓存时不应再写缓存，实际 %d 次", cache.sets)
	}
}

func TestGetUserInfo_CacheDownStillWorks(t *testing.T) {
	now := time.Now().Unix()
	uc, _, _, cache := newUsecase(t, now)
	cache.failGet = true

	// 缓存挂了不能让用户登不进来，必须降级查库
	info, err := uc.GetUserInfo(context.Background(), "tok-valid")
	if err != nil {
		t.Fatalf("缓存故障不应导致报错: %v", err)
	}
	if info == nil || info.UserID != 7 {
		t.Fatalf("缓存故障时应降级查库，实际 %+v", info)
	}
}

func TestSetUserInfo_ExpiredSessionIsInvalid(t *testing.T) {
	now := time.Now().Unix()
	uc, _, _, _ := newUsecase(t, now)

	info, err := uc.SetUserInfo(context.Background(), "tok-expired")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info != nil {
		t.Fatalf("过期会话必须判为无效，实际 %+v", info)
	}
}

// 这条锁住的是**与原实现的有意偏差**：
// ⚠️ 第 41 轮**反转了这条测试**。
//
// 原 PHP 在用户被软删除时会拼出 user_id=null 的「用户信息」并让鉴权通过。
// 我此前实现为"拒绝"，并写了这个测试要求必须拒绝。
// 按"不改动原项目逻辑"的要求已撤销那处收紧，所以本测试改为**锁住原行为**。
//
// ⚠️ 知情保留的安全后果：**账号被软删除后旧 token 仍能通过鉴权**。
// 这是原项目行为，不是迁移引入的。见 README 第八节。
func TestSetUserInfo_SoftDeletedUserFollowsOriginalPHP(t *testing.T) {
	now := time.Now().Unix()
	uc, sessions, users, cache := newUsecase(t, now)

	// 会话有效，但用户已不存在（软删除后的效果）
	sessions.byToken["tok-ghost"] = &UserSession{
		ID: 3, UserID: 999, Terminal: 3, Token: "tok-ghost",
		ExpireTime: now + 1800, UpdateTime: now,
	}
	delete(users.users, 999)

	info, err := uc.SetUserInfo(context.Background(), "tok-ghost")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}

	// 原实现：$user 为空也照样拼一份结构出来 -> 非空数组为真 -> 鉴权通过。
	// Go 侧对应 UserID 的零值 0（PHP 那边是 null）。
	if info == nil {
		t.Fatal("按原项目逻辑，用户行缺失时仍应返回（幽灵）用户信息并放行鉴权")
	}
	if info.UserID != 0 {
		t.Fatalf("幽灵用户的 UserID 应为 0（对应 PHP 的 null），实际 %d", info.UserID)
	}
	if info.Token != "tok-ghost" {
		t.Fatalf("token 应原样带上，实际 %q", info.Token)
	}
	if info.ExpireTime != now+1800 {
		t.Fatalf("过期时间应取自会话，实际 %d", info.ExpireTime)
	}
	if cache.sets == 0 {
		t.Fatal("按原实现应当写缓存（否则每次请求都会回源）")
	}
}

func TestOvertimeToken_ExtendsExpiry(t *testing.T) {
	now := time.Now().Unix()
	uc, sessions, _, _ := newUsecase(t, now)

	before := sessions.byToken["tok-valid"].ExpireTime
	info, err := uc.OvertimeToken(context.Background(), "tok-valid")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info == nil {
		t.Fatal("续期应成功")
	}
	want := now + int64((7 * 24 * time.Hour).Seconds())
	if info.ExpireTime != want {
		t.Fatalf("续期后过期时间 = %d, want %d", info.ExpireTime, want)
	}
	if info.ExpireTime <= before {
		t.Fatal("续期后过期时间必须变大")
	}
}

func TestOvertimeToken_UnknownTokenReturnsNil(t *testing.T) {
	now := time.Now().Unix()
	uc, _, _, _ := newUsecase(t, now)

	info, err := uc.OvertimeToken(context.Background(), "no-such-token")
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info != nil {
		t.Fatal("未知 token 续期应返回 nil（对应原实现返回 false）")
	}
}

func TestExpireToken(t *testing.T) {
	now := time.Now().Unix()
	uc, sessions, _, cache := newUsecase(t, now)

	ok, err := uc.ExpireToken(context.Background(), "tok-valid")
	if err != nil || !ok {
		t.Fatalf("登出失败: ok=%v err=%v", ok, err)
	}
	if sessions.expired != 1 {
		t.Fatalf("应当只更新 expire_time/update_time 一次，实际 %d", sessions.expired)
	}
	if cache.dels != 1 {
		t.Fatalf("应当删除缓存，实际 %d 次", cache.dels)
	}
	if sessions.byToken["tok-valid"].ExpireTime > now {
		t.Fatal("过期时间应被设为当前时间")
	}
}

func TestSetToken_RegeneratesExistingSession(t *testing.T) {
	now := time.Now().Unix()
	uc, sessions, _, _ := newUsecase(t, now)
	oldToken := sessions.byToken["tok-valid"].Token

	info, err := uc.SetToken(context.Background(), 7, 3)
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info == nil || info.Token == "" {
		t.Fatal("签发应返回带 token 的用户信息")
	}
	if info.Token == oldToken {
		t.Fatal("同一终端重新登录应当换发新 token")
	}
	if sessions.created != 0 {
		t.Fatal("已存在的会话应当更新而不是新建")
	}
	if sessions.updated != 1 {
		t.Fatalf("应当更新一次会话，实际 %d", sessions.updated)
	}
}

func TestSetToken_CreatesWhenAbsent(t *testing.T) {
	now := time.Now().Unix()
	uc, sessions, _, _ := newUsecase(t, now)

	info, err := uc.SetToken(context.Background(), 7, 9) // terminal=9 没有会话
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	if info == nil || info.Token == "" {
		t.Fatal("签发应返回带 token 的用户信息")
	}
	if sessions.created != 1 {
		t.Fatalf("应当新建一次会话，实际 %d", sessions.created)
	}
}

func TestCreateToken_Is32HexAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok := CreateToken("salt", "7")
		if len(tok) != 32 {
			t.Fatalf("token 长度应为 32（对齐 x_user_session.token varchar(32)），实际 %d: %s", len(tok), tok)
		}
		if _, err := hex.DecodeString(tok); err != nil {
			t.Fatalf("token 必须是十六进制: %s", tok)
		}
		if seen[tok] {
			t.Fatalf("重复 token: %s", tok)
		}
		seen[tok] = true
	}
}

func TestUserIDFromContext(t *testing.T) {
	ctx := context.Background()
	if got := UserIDFromContext(ctx); got != 0 {
		t.Fatalf("未登录时应为 0，实际 %d", got)
	}
	ctx = WithUserInfo(ctx, &UserInfo{UserID: 42})
	if got := UserIDFromContext(ctx); got != 42 {
		t.Fatalf("应为 42，实际 %d", got)
	}
	// nil 指针不能当成已登录
	ctx = WithUserInfo(context.Background(), nil)
	if _, ok := UserInfoFromContext(ctx); ok {
		t.Fatal("注入 nil 时不应被当作已登录")
	}
}
