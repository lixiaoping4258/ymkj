package biz

import (
	"context"
	"errors"
)

// AddressDeleteUsecase 对应 AddressLogic::delete($id, $userId)。
type AddressDeleteUsecase struct {
	repo AddressDeleteRepo
}

func NewAddressDeleteUsecase(repo AddressDeleteRepo) *AddressDeleteUsecase {
	return &AddressDeleteUsecase{repo: repo}
}

// AddressDeleteRepo 地址删除。
type AddressDeleteRepo interface {
	// ExistsOwned 对应 `UserAddress::where(['id'=>$id,'user_id'=>$userId])->find()`
	//
	// ⚠️ 条件里同时带 user_id -> **越权防护**（不能删别人的地址）。
	ExistsOwned(ctx context.Context, id int64, userID uint32) (bool, error)
	// HardDelete 对应 `$address->delete()`。
	//
	// ⚠️⚠️ 这是**硬删除**：`x_user_address` 表**没有 delete_time 列**
	//    （已对建表结构核实：id/user_id/app_id/contact_user/contact_phone/
	//     address/fettle/is_default），所以模型的 delete() 不会走软删除。
	//    行是**真的从表里消失**的，不可恢复。
	HardDelete(ctx context.Context, id int64, userID uint32) error
}

// ErrAddressDeleteNotFound 对应 `self::setError('未找到地址记录')`。
//
// ⚠️ 文案与 addressDefault 的"未找到地址记录"**相同**，但是两处独立设置。
var ErrAddressDeleteNotFound = errors.New("未找到地址记录")

// 逐字对应 UserAddressDeleteValidate::$message
var (
	ErrAddressIDRequired = errors.New("地址记录ID不能为空")
	ErrAddressIDInvalid  = errors.New("地址记录ID不正确")
)

// Delete 逐字对应 AddressLogic::delete($id, $userId)。
//
// 原文：
//
//	$address = UserAddress::where(['id'=>$id, 'user_id'=>$userId])->find();
//	if (!$address) { self::setError('未找到地址记录'); return false; }
//	return $address->delete();
//
// 控制器：
//
//	$params = (new UserAddressDeleteValidate())->get()->goCheck();
//	$result = AddressLogic::delete($params['id'], $this->userId);
//	if ($result) { return $this->success('删除成功'); }
//	else { return $this->fail(AddressLogic::getError()); }
//
// ⚠️⚠️ **这是用 GET 做删除**（`Route::get('address/delete', ...)`）。
//
//	迁移时必须按写操作对待：写权限、幂等、以及"被预取/爬虫误触发"的风险。
//
// ⚠️ 删除是**硬删除**（表无 delete_time），不可恢复。
func (uc *AddressDeleteUsecase) Delete(ctx context.Context, id int64, userID uint32) error {
	ok, err := uc.repo.ExistsOwned(ctx, id, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAddressDeleteNotFound
	}
	return uc.repo.HardDelete(ctx, id, userID)
}
