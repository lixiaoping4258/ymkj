package model

// MarketPurchase 对应 app\common\model\MarketPurchase，表 x_market_purchase。
//
// 这里只声明 Stage 3 用到的列。字段很多，按需补充 ——
// 用 Select() 明确列名，避免整行扫描时因为模型与老表列不一致而报错。
type MarketPurchase struct {
	ID     uint64 `gorm:"column:id;primaryKey"`
	UserID uint64 `gorm:"column:user_id"`
	// 业务状态，字符串：OFFLINE / WANTED / COMPLETE / EXPIRE
	// （另有一套数字的 ADMIN_STATE_* 给后台用，不要混）
	State string `gorm:"column:state"`
}
