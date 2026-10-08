package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type tradeCalendarRepo struct {
	db *gorm.DB
}

func NewTradeCalendarRepo(db *gorm.DB) biz.TradeCalendarRepo {
	return &tradeCalendarRepo{db: db}
}

// GetTypeByDate 对应 TradeCalendar::where([['date','=',$today]])->findOrEmpty()。
//
// 原实现只用了 type 字段，所以这里也只 Select("type")。
// 注意原代码是 `$record->type == 1 ? 'workday' : 'holiday'`，
// 任何不等于 1 的值（包括 NULL）都算节假日。
func (r *tradeCalendarRepo) GetTypeByDate(ctx context.Context, date string) (int, bool, error) {
	var row model.TradeCalendar
	err := r.db.WithContext(ctx).
		Model(&model.TradeCalendar{}).
		Select("type").
		Where("date = ?", date).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if row.Type == nil {
		return 0, true, nil
	}
	return *row.Type, true, nil
}
