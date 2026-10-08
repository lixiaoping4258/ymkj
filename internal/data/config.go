package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

// configRepo 对应 app/common/model/Config.php + ConfigService 的 DB 部分。
type configRepo struct {
	db *gorm.DB
}

func NewConfigRepo(db *gorm.DB) biz.ConfigRepo {
	return &configRepo{db: db}
}

// GetValue 取单条配置的原始 value。
//
// 只用 Select("value") 而不是 SELECT *：这张表是 likeadmin 的老表，
// 列可能和模型对不上，查需要的列最稳。
func (r *configRepo) GetValue(ctx context.Context, typ, name string) (any, bool, error) {
	var row model.Config
	err := r.db.WithContext(ctx).
		Model(&model.Config{}).
		Select("value").
		Where("type = ? AND name = ?", typ, name).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if row.Value == nil {
		return nil, true, nil // 记录存在但 value 是 NULL
	}
	return *row.Value, true, nil
}

// GetAll 取某个 type 下所有 name => value。
func (r *configRepo) GetAll(ctx context.Context, typ string) (map[string]any, error) {
	var rows []struct {
		Name  string  `gorm:"column:name"`
		Value *string `gorm:"column:value"`
	}
	err := r.db.WithContext(ctx).
		Model(&model.Config{}).
		Select("name, value").
		Where("type = ?", typ).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(rows))
	for _, row := range rows {
		if row.Value == nil {
			out[row.Name] = nil
			continue
		}
		out[row.Name] = *row.Value
	}
	return out, nil
}
