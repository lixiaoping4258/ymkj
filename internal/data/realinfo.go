package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// realInfoRepo 对应 app/common/model/user/UserReal.php。
type realInfoRepo struct {
	db     *gorm.DB
	prefix string
}

func NewRealInfoRepo(db *gorm.DB, c *conf.Data) biz.RealInfoRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &realInfoRepo{db: db, prefix: prefix}
}

// FindRealInfo 逐字对应：
//
//	UserReal::where(['user_id' => $userId])
//	    ->field('state,card_no,user_name,birthday,gender,address')
//	    ->findOrEmpty();
//
// ⚠️ 表里还有 verify_time / verify_count / seq / create_time，原文**只取这 6 列**。
// ⚠️ x_user_real **没有 delete_time** 列 -> 无软删除过滤。
// ⚠️ birthday / gender / address 可空 -> 用指针，NULL 时对外输出 null。
func (r *realInfoRepo) FindRealInfo(
	ctx context.Context, userID uint32,
) (*biz.RealInfoRow, bool, error) {
	var row struct {
		State    string  `gorm:"column:state"`
		CardNo   string  `gorm:"column:card_no"`
		UserName string  `gorm:"column:user_name"`
		Birthday *string `gorm:"column:birthday"`
		Gender   *int32  `gorm:"column:gender"`
		Address  *string `gorm:"column:address"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_real").
		Select("state", "card_no", "user_name", "birthday", "gender", "address").
		Where("user_id = ?", userID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.RealInfoRow{
		State: row.State, CardNo: row.CardNo, UserName: row.UserName,
		Birthday: row.Birthday, Gender: row.Gender, Address: row.Address,
	}, true, nil
}
