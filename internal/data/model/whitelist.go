package model

// WhitelistItem 对应 app\common\model\whitelist\WhitelistItem，表 x_whitelist_item。
type WhitelistItem struct {
	ID     uint64 `gorm:"column:id;primaryKey"`
	Code   string `gorm:"column:code"`
	Status int32  `gorm:"column:status"`
}

// WhitelistGroup 表 x_whitelist_group。
type WhitelistGroup struct {
	ID     uint64 `gorm:"column:id;primaryKey"`
	Status int32  `gorm:"column:status"`
}

// WhitelistGroupItem 表 x_whitelist_group_item：某个组对某个限制项的开关。
//
// enabled != 0 表示该组启用了这条限制（也就是"禁用该操作"）。
type WhitelistGroupItem struct {
	GroupID uint64 `gorm:"column:group_id"`
	ItemID  uint64 `gorm:"column:item_id"`
	Enabled int32  `gorm:"column:enabled"`
}

// WhitelistUser 表 x_whitelist_user：用户属于哪个白名单组（一个用户只在一个组）。
type WhitelistUser struct {
	UserID  uint64 `gorm:"column:user_id"`
	GroupID uint64 `gorm:"column:group_id"`
}
