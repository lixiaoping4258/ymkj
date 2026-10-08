package data

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

/* ---------------------------------------------------------------- 裸 Redis 缓存 */

// rawRedisCache 对应原项目 RedisLockService 的 get/set：
// 直接操作 phpredis，**没有前缀**，值就是 json_encode 的结果。
//
// ⚠️ 因此这个缓存是**和 PHP 共用**的：PHP 写进去的 Go 能读，反之亦然。
// 共用是有意为之 —— 两边都受益于对方已预热的缓存。
// 前提是 JSON 形状必须逐字一致，所以 biz 层组装 map 时字段名/类型都对着 PHP 写死。
//
// 与它相反的是 ThinkPHP 的 cache()（带 la: 前缀 + TP 序列化），那类缓存
// Go 侧必须用隔离前缀自己管一份。
type rawRedisCache struct {
	cli *redis.Client
}

func NewRawRedisCache(cli *redis.Client) biz.RawCache {
	return &rawRedisCache{cli: cli}
}

func (c *rawRedisCache) Get(ctx context.Context, key string) (json.RawMessage, bool, error) {
	b, err := c.cli.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(b), true, nil
}

func (c *rawRedisCache) Set(ctx context.Context, key string, val json.RawMessage, ttl time.Duration) error {
	if len(val) == 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Second
	}
	return c.cli.Set(ctx, key, []byte(val), ttl).Err()
}

/* ---------------------------------------------------------------- 档案仓储 */

type archiveRepo struct {
	db     *gorm.DB
	prefix string
}

// NewArchiveRepo 需要显式传前缀：FindArchive 用的是 Table()+字符串拼表名，
// 而 GORM 的 Table() **不会**套用 NamingStrategy 的 TablePrefix ——
// 传 nil 就会去查 app_archive（不存在），报 Table doesn't exist。
func NewArchiveRepo(db *gorm.DB, c *conf.Data) biz.ArchiveRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &archiveRepo{db: db, prefix: prefix}
}

// archiveScan 的列名对应 SQL 的输出别名。
type archiveScan struct {
	ID           int64   `gorm:"column:id"`
	CollectionID string  `gorm:"column:collection_id"`
	Name         string  `gorm:"column:name"`
	Images       *string `gorm:"column:images"`
	Issuer       string  `gorm:"column:issuer"`
	PlatformName string  `gorm:"column:platform_name"`
	PlatformID   int64   `gorm:"column:platform_id"`
}

// FindArchive 的 SQL 是用 PHP CLI 的 fetchSql(true) 实测出来的：
//
//	SELECT `app_archive`.`id`,`app_archive`.`collection_id`,`app_archive`.`name`,
//	       `app_archive`.`images`,`app_archive`.`issuer`,
//	       app.name platform_name, app.id platform_id
//	FROM `x_app_archive` `app_archive`
//	INNER JOIN `x_app` `app` ON `app`.`id`=`app_archive`.`app_id`
//	WHERE `app_archive`.`id` = ? AND `app_archive`.`state` = '1' LIMIT 1
//
// 注意 platform_name / platform_id 是 ThinkPHP 的隐式别名写法（没写 as），
// 输出列名就是 platform_name / platform_id。
func (r *archiveRepo) FindArchive(ctx context.Context, id int64) (*biz.ArchiveRow, error) {
	var row archiveScan
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_archive AS app_archive").
		Select("app_archive.id, app_archive.collection_id, app_archive.name, "+
			"app_archive.images, app_archive.issuer, "+
			"app.name AS platform_name, app.id AS platform_id").
		Joins("INNER JOIN "+r.prefix+"app AS app ON app.id = app_archive.app_id").
		Where("app_archive.id = ? AND app_archive.state = ?", id, 1).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.ArchiveRow{
		ID:           row.ID,
		CollectionID: row.CollectionID,
		Name:         row.Name,
		Images:       row.Images,
		Issuer:       row.Issuer,
		PlatformName: row.PlatformName,
		PlatformID:   row.PlatformID,
	}, nil
}

// FindSaleArchive 对应 SaleLogic::getArchive 的查询。
//
// ⚠️ 与上面的 FindArchive **只差两列**（少了 collection_id 和 platform_id），
// 但两者是不同函数、不同缓存机制、不同 TTL。抄的时候别"顺手统一"。
//
//	SELECT `app_archive`.`id`,`app_archive`.`name`,`app_archive`.`images`,
//	       `app_archive`.`issuer`, app.name platform_name
//	FROM `x_app_archive` `app_archive`
//	INNER JOIN `x_app` `app` ON `app`.`id`=`app_archive`.`app_id`
//	WHERE `app_archive`.`id` = ? AND `app_archive`.`state` = '1' LIMIT 1
func (r *archiveRepo) FindSaleArchive(ctx context.Context, id int64) (*biz.SaleArchiveRow, error) {
	var row struct {
		ID           int64   `gorm:"column:id"`
		Name         string  `gorm:"column:name"`
		Images       *string `gorm:"column:images"`
		Issuer       string  `gorm:"column:issuer"`
		PlatformName string  `gorm:"column:platform_name"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"app_archive AS app_archive").
		Select("app_archive.id, app_archive.name, app_archive.images, app_archive.issuer, "+
			"app.name AS platform_name").
		Joins("INNER JOIN "+r.prefix+"app AS app ON app.id = app_archive.app_id").
		Where("app_archive.id = ? AND app_archive.state = ?", id, 1).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.SaleArchiveRow{
		ID:           row.ID,
		Name:         row.Name,
		Images:       row.Images,
		Issuer:       row.Issuer,
		PlatformName: row.PlatformName,
	}, nil
}

// FindSalesArchiveID 对应 MarketListSales::where(['id'=>$id])->field('id,archive_id')。
// x_market_list_sales 没有 delete_time 列，不需要软删除过滤。
func (r *archiveRepo) FindSalesArchiveID(ctx context.Context, salesID int64) (int64, bool, error) {
	var row struct {
		ArchiveID int64 `gorm:"column:archive_id"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"market_list_sales").
		Select("archive_id").
		Where("id = ?", salesID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return row.ArchiveID, true, nil
}
