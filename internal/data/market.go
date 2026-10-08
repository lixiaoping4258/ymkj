package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type marketPurchaseRepo struct {
	db *gorm.DB
}

func NewMarketPurchaseRepo(db *gorm.DB) biz.MarketPurchaseRepo {
	return &marketPurchaseRepo{db: db}
}

// CountByUserAndState 对应：
//
//	MarketPurchase::where(["user_id" => $userId, "state" => MarketPurchaseEnum::STATE_WANTED])->count()
//
// state 是字符串（'WANTED' 等），不是数字。
func (r *marketPurchaseRepo) CountByUserAndState(ctx context.Context, userID uint64, state string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.MarketPurchase{}).
		Where("user_id = ? AND state = ?", userID, state).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}
