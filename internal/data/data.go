package data

import (
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// ProviderSet 是 data 层的依赖注入集合。
var ProviderSet = wire.NewSet(
	NewDB,
	NewRedis,
	NewRedisCache,
	NewConfigRepo,
	NewTradeCalendarRepo,
	NewProjectConfig,
	NewLocation,
	NewAuthConfig,
	// userRepo 同时实现了两个接口，用 wire.Bind 各绑一次。
	// 注意 NewUserRepo 必须返回具体类型，否则这里绑不了。
	NewUserRepo,
	wire.Bind(new(biz.UserProfileRepo), new(*userRepo)),
	wire.Bind(new(biz.UserBriefRepo), new(*userRepo)),
	NewUserSessionRepo,
	NewUserTokenCache,
	// 锁的键与 PHP 共用（不加隔离前缀），见 data/lock.go 的说明
	NewRedisLocker,
	NewMarketPurchaseRepo,
	NewWhitelistRepo,
	NewPurchaseFaceRepo,
	NewAppFlags,
)

// gormWriter 把 GORM 的 SQL 日志转接到 Kratos 的 logger。
//
// 不这么做的话 GORM 会直接往 stdout 打，日志格式和 Kratos 的
// 结构化输出混在一起，采集和检索都难受。
type gormWriter struct {
	l *log.Helper
}

func (w gormWriter) Printf(format string, args ...interface{}) {
	w.l.Infof(strings.TrimSuffix(format, "\n"), args...)
}

// NewDB 建 MySQL 连接。
//
// 表名策略必须和 ThinkPHP 对齐：
//   - TablePrefix  = 原项目 .env 里 DATABASE.PREFIX（**是 x_，不是 database.php 的默认 la_**）
//   - SingularTable = true，否则 GORM 会把 Config 复数成 configs
func NewDB(c *conf.Data, logger log.Logger) (*gorm.DB, func(), error) {
	l := log.NewHelper(logger)

	prefix := "la_"
	if c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}

	// IgnoreRecordNotFoundError 必须开：本项目大量用「查不到记录」当作正常分支
	// （比如交易日历没标今天 → 回退到按周几判断）。不开的话这些预期分支
	// 会在 Warn 级别打出一堆 "record not found"，把真正的错误淹掉。
	gcfg := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   prefix,
			SingularTable: true,
		},
		Logger: gormlogger.New(gormWriter{l: l}, gormlogger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	}
	if c.Database != nil && c.Database.Debug {
		gcfg.Logger = gormlogger.New(gormWriter{l: l}, gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormlogger.Info,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		})
	}

	source := ""
	if c.Database != nil {
		source = c.Database.Source
	}
	if source == "" {
		l.Warn("database.source 为空，请检查 configs/config.yaml")
	}

	db, err := gorm.Open(mysql.Open(source), gcfg)
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}

	maxOpen := 100
	if c.Database != nil && c.Database.MaxOpenConns > 0 {
		maxOpen = int(c.Database.MaxOpenConns)
	}
	maxIdle := 20
	if c.Database != nil && c.Database.MaxIdleConns > 0 {
		maxIdle = int(c.Database.MaxIdleConns)
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	// 比 MySQL 的 wait_timeout 短，避免用到被服务端单方面关闭的连接
	sqlDB.SetConnMaxLifetime(time.Hour)

	cleanup := func() {
		l.Info("关闭 MySQL 连接")
		_ = sqlDB.Close()
	}
	l.Infof("MySQL 已连接 (prefix=%s, maxOpen=%d)", prefix, maxOpen)
	return db, cleanup, nil
}

// NewRedis 建 Redis 连接。
func NewRedis(c *conf.Data, logger log.Logger) (*redis.Client, func(), error) {
	l := log.NewHelper(logger)
	if c.Redis == nil {
		l.Warn("未配置 redis，缓存相关功能会退化为不可用")
	}
	rc := c.Redis
	cli := redis.NewClient(&redis.Options{
		Addr:         rc.GetAddr(),
		Password:     rc.GetPassword(),
		DB:           int(rc.GetDb()),
		ReadTimeout:  rc.GetReadTimeout().AsDuration(),
		WriteTimeout: rc.GetWriteTimeout().AsDuration(),
	})
	cleanup := func() {
		l.Info("关闭 Redis 连接")
		_ = cli.Close()
	}
	l.Infof("Redis 已连接 (%s db=%d)", rc.GetAddr(), rc.GetDb())
	return cli, cleanup, nil
}

// NewLocation 加载时区。
//
// 时区直接影响「交易时段」判断（date('H:i') / date('N') / 当天 24 点缓存过期），
// 配错会导致开市判断整体偏移，务必和原项目 .env 的 DEFAULT_TIMEZONE 一致。
func NewLocation(c *conf.App, logger log.Logger) *time.Location {
	l := log.NewHelper(logger)
	name := "Asia/Shanghai"
	if c != nil && c.Timezone != "" {
		name = c.Timezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		l.Errorf("时区 %s 加载失败(%v)，回退到 UTC —— 交易时段判断会偏移", name, err)
		return time.UTC
	}
	l.Infof("时区: %s", name)
	return loc
}
