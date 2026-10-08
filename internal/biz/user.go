package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 本文件对应原项目：
//   app/api/controller/v1/user/UserController::info
//   app/api/logic/UserLogic::info

// x_user_accounts.app_id 的取值。原代码写的是魔法数字 3 / 4。
const (
	AppIDTea = 3 // 茶平台
	AppIDTao = 4 // 陶平台
)

// UserRow 是 x_user 里 info 需要的字段（原代码 field() 明确列了这些）。
type UserRow struct {
	ID         uint64
	Sn         uint64
	Sex        int32
	Nickname   string
	RealName   string
	Avatar     string
	Mobile     string
	CreateTime uint64
	// decimal(10,2)，且该列声明为**可空**。
	// ThinkPHP 对 decimal 输出字符串 "0.00"，NULL 时输出 null —— 所以用指针保留 NULL。
	UserMoney *string
	// password 会被 hidden 掉，不出现在响应里；只用来算 has_password。
	// 该列 NOT NULL，所以用普通 string。
	Password string
	// opt_pwd（支付密码哈希）原项目**没有** hidden，直接返回给前端了。
	// 见 README 安全章节。该列可空（实测 93% 的用户是 NULL）。
	OptPwd *string
}

// UserProfile 是 UserLogic::info 组装出的 $profile。
type UserProfile struct {
	ID         uint64
	Sn         uint64
	Sex        int32
	Nickname   string
	RealName   string
	Avatar     string
	Mobile     string
	CreateTime uint64
	// 可空：NULL 时必须序列化成 null，不能是 ""
	UserMoney    *string
	OptPwd       *string
	HasPassword  bool
	HasOptPwd    bool
	IsReal       bool
	UserCode     string
	UserCodeAuth UserCodeAuth
}

type UserCodeAuth struct {
	TeaUserCode string
	TaoUserCode string
}

// UserProfileRepo 是 info 需要的数据访问。
type UserProfileRepo interface {
	FindProfile(ctx context.Context, userID uint64) (*UserRow, error)
	// IsRealVerified 对应 UserReal where user_id=? and state='SUCCESS'
	IsRealVerified(ctx context.Context, userID uint64) (bool, error)
	// FindUserCode 对应 UserAccounts where user_id=? and app_id=? -> user_code
	FindUserCode(ctx context.Context, userID uint64, appID int) (string, error)
}

// UserUsecase 用户域用例。
type UserUsecase struct {
	repo  UserProfileRepo
	cache Cache
	log   *log.Helper
}

func NewUserUsecase(repo UserProfileRepo, cache Cache, logger log.Logger) *UserUsecase {
	return &UserUsecase{repo: repo, cache: cache, log: log.NewHelper(logger)}
}

// Info 对应 UserLogic::info($userId)。
//
// 原实现有一层 5 秒缓存（RedisLockService，裸 Redis 存 json_encode 结果）。
// 这里复用 biz.Cache，但**键空间是隔离的**（带 xtravel:go: 前缀）——
// 原键 user:info:{id} 是裸 Redis 写入的、内容是纯 JSON，理论上可以共用，
// 但共用意味着两边的 JSON 形状必须逐字段一致，任何一边调整都会串味。
// 迁移期先隔离，PHP 下线后再谈合并。
func (uc *UserUsecase) Info(ctx context.Context, userID uint64) (*UserProfile, error) {
	if userID == 0 {
		return nil, nil
	}
	cacheKey := fmt.Sprintf("user:info:%d", userID)

	if v, err := uc.cache.Get(ctx, cacheKey); err != nil {
		uc.log.WithContext(ctx).Warnf("读取 user:info 缓存失败: %v", err)
	} else if s, ok := v.(string); ok {
		var p UserProfile
		if err := json.Unmarshal([]byte(s), &p); err == nil {
			return &p, nil
		}
		// 脏缓存，忽略并继续回源
	}

	row, err := uc.repo.FindProfile(ctx, userID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}

	profile := &UserProfile{
		ID:         row.ID,
		Sn:         row.Sn,
		Sex:        row.Sex,
		Nickname:   row.Nickname,
		RealName:   row.RealName,
		Avatar:     row.Avatar,
		Mobile:     row.Mobile,
		CreateTime: row.CreateTime,
		UserMoney:  row.UserMoney,
		OptPwd:     row.OptPwd,
		// PHP: !empty($password) / !empty($opt_pwd)
		// 必须用 PhpTruthy 而不是「长度 > 0」：PHP 里 empty("0") 也是真，
		// 所以值为字符串 "0" 时 has_* 应当是 false。
		HasPassword: PhpTruthy(row.Password),
		HasOptPwd:   row.OptPwd != nil && PhpTruthy(*row.OptPwd),
	}

	// 实名状态：只有 state='SUCCESS' 记录存在才算已实名
	verified, err := uc.repo.IsRealVerified(ctx, userID)
	if err != nil {
		return nil, err
	}
	profile.IsReal = verified

	// 平台用户编号：茶(app_id=3) / 陶(app_id=4)
	teaCode, err := uc.repo.FindUserCode(ctx, userID, AppIDTea)
	if err != nil {
		return nil, err
	}
	taoCode, err := uc.repo.FindUserCode(ctx, userID, AppIDTao)
	if err != nil {
		return nil, err
	}
	// PHP: empty($userCode) ? '' : $userCode
	if teaCode == "0" {
		teaCode = ""
	}
	profile.UserCode = teaCode
	profile.UserCodeAuth = UserCodeAuth{TeaUserCode: teaCode, TaoUserCode: taoCode}

	if b, err := json.Marshal(profile); err == nil {
		if err := uc.cache.Set(ctx, cacheKey, string(b), 5*time.Second); err != nil {
			uc.log.WithContext(ctx).Warnf("写入 user:info 缓存失败: %v", err)
		}
	}
	return profile, nil
}
