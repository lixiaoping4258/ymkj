package data

import (
	"context"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// payChannelRepo 对应 PaymentLogic::getScenePayWay 的单条查询。
type payChannelRepo struct {
	db     *gorm.DB
	prefix string
}

func NewPayChannelRepo(db *gorm.DB, c *conf.Data) biz.PayChannelRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &payChannelRepo{db: db, prefix: prefix}
}

// ListScenePayWay 逐字对应：
//
//	PayWay::hasWhere('payConfig',
//	        ['PayWay.scene'=>$scene, 'PayWay.status'=>YesNoEnum::YES],
//	        'PayWay.id,PayWay.is_default,PayConfig.name,PayConfig.icon,
//	         PayConfig.remark,PayConfig.need_pass')
//	    ->orderRaw('PayConfig.sort desc')->select()->toArray();
//
// ⚠️⚠️ 表名是 **x_dev_pay_way / x_dev_pay_config**（模型里 `protected $name = 'dev_pay_way'`），
//
//	按类名猜成 x_pay_way 会报 "Table doesn't exist"。
//
// ⚠️ INNER JOIN（ThinkPHP 的 hasWhere 默认）—— pay_config_id 查不到配置的行不出现。
// ⚠️ 排序只按 `PayConfig.sort DESC`，**没有第二排序键** ——
//
//	同 sort 的行顺序由 MySQL 决定（不稳定）。照搬，不额外加 id 排序。
func (r *payChannelRepo) ListScenePayWay(ctx context.Context, scene int32) ([]biz.PayChannelRow, error) {
	var rows []struct {
		ID        int64  `gorm:"column:id"`
		IsDefault int32  `gorm:"column:is_default"`
		Name      string `gorm:"column:name"`
		Icon      string `gorm:"column:icon"`
		Remark    string `gorm:"column:remark"`
		NeedPass  int32  `gorm:"column:need_pass"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"dev_pay_way AS PayWay").
		Joins("INNER JOIN "+r.prefix+"dev_pay_config AS PayConfig ON PayConfig.id = PayWay.pay_config_id").
		Select("PayWay.id AS id, PayWay.is_default AS is_default, "+
			"PayConfig.name AS name, PayConfig.icon AS icon, "+
			"PayConfig.remark AS remark, PayConfig.need_pass AS need_pass").
		Where("PayWay.scene = ?", scene).
		Where("PayWay.status = ?", biz.PayWayStatusEnabled).
		Order("PayConfig.sort DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]biz.PayChannelRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, biz.PayChannelRow{
			ID: x.ID, IsDefault: x.IsDefault,
			Name: x.Name, Icon: x.Icon, Remark: x.Remark, NeedPass: x.NeedPass,
		})
	}
	return out, nil
}
