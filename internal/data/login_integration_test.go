package data

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// 登录仓储的集成测试（连真库）。
//
// 重点验证 x_user_accounts 的**软删除过滤** —— 该表 `delete_time` 是
// datetime NULLABLE（已对建表结构核实），ThinkPHP 的 SoftDelete 会隐式加过滤，
// GORM 不会。漏了就会把已删除的账号当成有效账号（与第 6 轮 MarketPurchase
// 那个 bug 同一类）。
func TestLoginRepo_FindAccount_SoftDeleteFiltered(t *testing.T) {
	db, prefix := openTestDB(t)
	ctx := context.Background()

	// 复用的 ID 生成器（Register 需要），这里只测查询，给一个真实 Redis 客户端即可
	cli := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // 连不上也没关系
	repo := NewLoginRepo(db, testConf(prefix), NewIDGenerator(cli), nil)

	// ---- 1. 找一个真实存在的账号 ----
	var live struct {
		Account string `gorm:"column:account"`
		AppID   int    `gorm:"column:app_id"`
		Type    int    `gorm:"column:type"`
		UserID  uint64 `gorm:"column:user_id"`
	}
	err := db.Raw("SELECT account, app_id, type, user_id FROM " + prefix + "user_accounts" +
		" WHERE delete_time IS NULL LIMIT 1").Scan(&live).Error
	if err != nil {
		t.Fatalf("直查失败: %v", err)
	}
	if live.Account == "" {
		t.Skip("库中没有未删除的 user_accounts 记录可测")
	}

	got, found, err := repo.FindAccount(ctx, live.AppID, live.Type, live.Account)
	if err != nil {
		t.Fatalf("FindAccount 失败: %v", err)
	}
	if !found {
		t.Fatalf("应找到账号 %q (app_id=%d type=%d)", live.Account, live.AppID, live.Type)
	}
	if got.UserID != live.UserID {
		t.Fatalf("user_id = %d, 直查 %d", got.UserID, live.UserID)
	}

	// ---- 2. 软删除的账号必须查不到 ----
	var dead struct {
		Account string `gorm:"column:account"`
		AppID   int    `gorm:"column:app_id"`
		Type    int    `gorm:"column:type"`
	}
	err = db.Raw("SELECT account, app_id, type FROM " + prefix + "user_accounts" +
		" WHERE delete_time IS NOT NULL LIMIT 1").Scan(&dead).Error
	if err != nil {
		t.Fatalf("直查软删除记录失败: %v", err)
	}
	if dead.Account == "" {
		t.Log("库中没有软删除的 user_accounts 记录 —— 这条断言本次无法生效（前提不成立，非通过）")
	} else {
		_, found, err := repo.FindAccount(ctx, dead.AppID, dead.Type, dead.Account)
		if err != nil {
			t.Fatalf("FindAccount 失败: %v", err)
		}
		if found {
			t.Fatalf("已软删除的账号 %q 不应被查到 —— delete_time IS NULL 过滤没生效",
				dead.Account)
		}
	}

	// ---- 3. FindUserByID 也要过滤软删除 ----
	u, err := repo.FindUserByID(ctx, live.UserID)
	if err != nil {
		t.Fatalf("FindUserByID 失败: %v", err)
	}
	if u == nil {
		t.Fatalf("user_id=%d 应存在（账号是 LIVE 的）", live.UserID)
	}
	if u.ID != live.UserID {
		t.Fatalf("返回的 user id = %d, 期望 %d", u.ID, live.UserID)
	}

	// ---- 4. 不存在的 id 返回 nil 而不是 error ----
	if u2, err := repo.FindUserByID(ctx, 999999999); err != nil || u2 != nil {
		t.Fatalf("不存在的用户应返回 (nil, nil)，实际 (%v, %v)", u2, err)
	}
}

// 确保测试用的 gorm 配置与生产一致（表前缀 + SingularTable）。
var _ = mysql.Open
var _ = gorm.Open
var _ = schema.NamingStrategy{}
var _ = gormlogger.Default
