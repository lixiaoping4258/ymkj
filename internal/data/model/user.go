package model

// User 对应 app\common\model\user\User，表 x_user。
//
// ⚠️ 这个模型用了 SoftDelete（delete_time）。GORM 的 gorm.DeletedAt 是给
// datetime 列用的，而这里是 int unsigned NULL，对不上，所以不用 GORM 的软删除，
// 改在查询里显式加 `delete_time IS NULL`。
type User struct {
	ID         uint64  `gorm:"column:id;primaryKey"`
	Sn         uint64  `gorm:"column:sn"`
	Sex        int32   `gorm:"column:sex"`
	Nickname   string  `gorm:"column:nickname"`
	RealName   string  `gorm:"column:real_name"`
	Avatar     string  `gorm:"column:avatar"`
	Mobile     string  `gorm:"column:mobile"`
	CreateTime uint64  `gorm:"column:create_time"`
	UserMoney  string  `gorm:"column:user_money"` // decimal(10,2)，PHP 输出字符串
	Password   string  `gorm:"column:password"`
	OptPwd     string  `gorm:"column:opt_pwd"`
	DeleteTime *uint64 `gorm:"column:delete_time"`
}

// UserSession 对应 app\common\model\user\UserSession，表 x_user_session。
// 没有软删除。
//
// 注意 expire_time / update_time 是 int 时间戳（不是 datetime）。
type UserSession struct {
	ID         uint64 `gorm:"column:id;primaryKey"`
	UserID     uint64 `gorm:"column:user_id"`
	Terminal   int32  `gorm:"column:terminal"`
	Token      string `gorm:"column:token"`
	UpdateTime int64  `gorm:"column:update_time"`
	ExpireTime int64  `gorm:"column:expire_time"`
}

// UserReal 对应 app\common\model\user\UserReal，表名显式声明为 user_real。
// 没有软删除。
type UserReal struct {
	UserID uint64 `gorm:"column:user_id"`
	State  string `gorm:"column:state"`
}

// UserAccounts 对应 app\common\model\user\UserAccounts，表 user_accounts。
// **有** SoftDelete（delete_time）。
type UserAccounts struct {
	ID         uint64  `gorm:"column:id;primaryKey"`
	UserID     uint64  `gorm:"column:user_id"`
	AppID      int32   `gorm:"column:app_id"`
	UserCode   string  `gorm:"column:user_code"`
	DeleteTime *uint64 `gorm:"column:delete_time"`
}
