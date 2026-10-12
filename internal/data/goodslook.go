package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// goodsLookRepo / goodsLookArchiveRepo 对应 GoodsLogic::goodsLook 的两段查询。
type goodsLookRepo struct {
	db     *gorm.DB
	prefix string
}

func NewGoodsLookRepo(db *gorm.DB, c *conf.Data) *goodsLookRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &goodsLookRepo{db: db, prefix: prefix}
}

// ListArchives 逐字对应：
//
//	$whereAppArchive = ["state" => 1];
//	if (isset($params['archive_id'])) { $whereAppArchive['id'] = trim($params['archive_id']); }
//	AppArchive::where($whereAppArchive)
//	    ->field("app_id,images as goods_image,name as goods_name,
//	            collection_id,id as archive_id")->select();
//
// ⚠️ **5 个字段带别名**：`images as goods_image`、`name as goods_name`、`id as archive_id`。
// ⚠️ `isset` 语义：传了空串也会加 `id = ”`（查不到任何行）—— 用 hasArchiveID 区分。
// ⚠️ x_app_archive **没有 delete_time** -> 不加软删除过滤。
// ⚠️ 没有 order（原文没写）—— 顺序由 MySQL 决定。
func (r *goodsLookRepo) ListArchives(
	ctx context.Context, archiveID string, hasArchiveID bool,
) ([]map[string]any, error) {
	tx := r.db.WithContext(ctx).
		Table(r.prefix+"app_archive").
		Select("app_id AS app_id, images AS goods_image, name AS goods_name, "+
			"collection_id AS collection_id, id AS archive_id").
		Where("state = ?", 1)
	if hasArchiveID {
		tx = tx.Where("id = ?", archiveID)
	}
	var rows []map[string]any
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, m := range rows {
		biz.FormatTimeValuesInMap(m)
	}
	return rows, nil
}

// CountByCollection 逐字对应：
//
//	WarehouseDetail::where(['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0])
//	    ->whereIn('collection_id', $collectionIds)
//	    ->field('collection_id, COUNT(*) as num')->group('collection_id')->select();
//
// ⚠️ 表 x_warehouse_detail 无软删除（WarehouseDetail 未 use SoftDelete）。
func (r *goodsLookRepo) CountByCollection(
	ctx context.Context, userID uint32, collectionIDs []int64,
) (map[int64]int64, error) {
	var rows []struct {
		CollectionID int64 `gorm:"column:collection_id"`
		Num          int64 `gorm:"column:num"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"warehouse_detail").
		Select("collection_id, COUNT(*) AS num").
		Where("user_id = ?", userID).
		Where("is_locked = ?", 1).
		Where("used = ?", 0).
		Where("is_use_num = ?", 0).
		Where("collection_id IN ?", collectionIDs).
		Group("collection_id").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int64, len(rows))
	for _, x := range rows {
		out[x.CollectionID] = x.Num
	}
	return out, nil
}
