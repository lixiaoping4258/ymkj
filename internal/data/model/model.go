// Package model 放 GORM 模型。
//
// 表名策略：**不要**给模型实现 TableName()。
// GORM 只有在模型没实现 Tabler 时才会套用 NamingStrategy 的 TablePrefix，
// 自己实现 TableName() 会让 la_ 前缀失效。
// 配合 data.go 里的 NamingStrategy{SingularTable: true}，
// Config -> la_config、TradeCalendar -> la_trade_calendar，
// 与 ThinkPHP 的「前缀 + 蛇形单数」规则一致。
package model

// Config 对应 ThinkPHP 的 app\common\model\Config，表 la_config。
type Config struct {
	ID    uint    `gorm:"column:id;primaryKey"`
	Type  string  `gorm:"column:type"`
	Name  string  `gorm:"column:name"`
	Value *string `gorm:"column:value"` // 可为 NULL
}

// TradeCalendar 对应 app\common\model\TradeCalendar，表 la_trade_calendar。
// 人工标记某天是工作日还是节假日。
type TradeCalendar struct {
	ID   uint   `gorm:"column:id;primaryKey"`
	Date string `gorm:"column:date"`
	// 1 = 强制工作日，2(或其它) = 强制节假日。
	// 原实现是 $record->type == 1 ? 'workday' : 'holiday'，所以只要不等于 1 都算节假日。
	Type *int `gorm:"column:type"`
}
