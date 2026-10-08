package model

// WarehouseDetail 对应 app\common\model\warehouse\WarehouseDetail，表 x_warehouse_detail。
//
// 该模型继承的是 think\Model（不是 BaseModel），显式声明了
// `protected $name = 'warehouse_detail'`，且表里**没有 delete_time 列**
// （已核对 information_schema），所以查询不需要软删除过滤。
type WarehouseDetail struct {
	ID       uint64 `gorm:"column:id;primaryKey"`
	UserID   uint64 `gorm:"column:user_id"`
	IsLocked int32  `gorm:"column:is_locked"`
	Used     int32  `gorm:"column:used"`
	IsUseNum int32  `gorm:"column:is_use_num"`
}
