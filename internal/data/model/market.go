package model

import "time"

// MarketPurchase 对应 app\common\model\MarketPurchase，表 x_market_purchase。
//
// ⚠️ 这个模型**用了 SoftDelete**（`use SoftDelete; protected $deleteTime = 'delete_time';`），
// 所以所有查询都必须带上 `delete_time IS NULL`，否则会把已软删除的记录也算进来。
//
// 注意本表的 delete_time 是 **datetime**（可空），而 x_user 的是 int unsigned ——
// 判空条件写法一样，但类型不同，模型里别混用。
//
// 这一点是从 PurchaseStateLists 源码第 90 行的注释里发现的（那段注释贴了真实 SQL）：
//
//	... AND `MarketPurchase`.`delete_time` IS NULL ORDER BY `MarketPurchase`.`id` DESC LIMIT 0,25
//
// 实测：x_market_purchase 共 750 行，其中 65 行是软删除的。
type MarketPurchase struct {
	ID     uint64 `gorm:"column:id;primaryKey"`
	UserID uint64 `gorm:"column:user_id"`
	// 业务状态，ENUM：OFFLINE / WANTED / COMPLETE / EXPIRE
	// （另有一套数字的 ADMIN_STATE_* 给后台用，不要混）
	State string `gorm:"column:state"`
	// 软删除标记。非 nil 表示已删除。
	DeleteTime *time.Time `gorm:"column:delete_time"`
}

// NotDeletedMktPurchase 对应 ThinkPHP SoftDelete 的隐式过滤条件。
// 导出是为了让 data 层各处查询统一引用同一份字面量，避免有人漏写。
const NotDeletedMktPurchase = "delete_time IS NULL"
