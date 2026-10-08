package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type stockRepo struct {
	db *gorm.DB
}

func NewStockRepo(db *gorm.DB) biz.StockRepo {
	return &stockRepo{db: db}
}

// CountUnlockedStock 对应：
//
//	WarehouseDetail::where(['user_id'=>$userId,'is_locked'=>1,'used'=>0,'is_use_num'=>0])->count()
//
// x_warehouse_detail 没有 delete_time 列（已核对 information_schema），
// 且模型继承的是 think\Model 而非 BaseModel，所以**没有软删除过滤**要加。
func (r *stockRepo) CountUnlockedStock(ctx context.Context, userID uint64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.WarehouseDetail{}).
		Where("user_id = ? AND is_locked = ? AND used = ? AND is_use_num = ?", userID, 1, 0, 0).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// UnlockInProgress 见 biz.StockRepo 接口上的说明。
//
// 迁移期固定返回 false —— 这个标记由 PHP 用 ThinkPHP 的 cache() 写，
// 键空间和序列化格式都与 Go 侧隔离，读不到就不要假装读到。
// 返回 true 会凭空让 state 变 0（前端误以为有任务在处理），
// 宁可保守返回 false，行为退化为"永远显示正常"。
func (r *stockRepo) UnlockInProgress(_ context.Context, _ uint64) (bool, error) {
	return false, nil
}
