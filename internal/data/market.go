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
// ⚠️ 必须带 `delete_time IS NULL`：MarketPurchase 用了 SoftDelete，
// ThinkPHP 会自动加这个条件，GORM 不会。漏掉的话软删除的兑换单也会被算进来，
// checkExchange 就会错误地拦住用户（"当前存在兑换单，暂无法导入数字艺术品"）。
// 实测该表 750 行里有 65 行是软删除的。
func (r *marketPurchaseRepo) CountByUserAndState(ctx context.Context, userID uint64, state string) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.MarketPurchase{}).
		Where("user_id = ? AND state = ?", userID, state).
		Where(model.NotDeletedMktPurchase).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}
