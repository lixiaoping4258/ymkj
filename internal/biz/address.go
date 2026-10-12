package biz

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// AddressUsecase 对应 app/api/logic/user/AddressLogic.php。
type AddressUsecase struct {
	repo  AddressRepo
	cache RawKeyCache
}

func NewAddressUsecase(repo AddressRepo, cache RawKeyCache) *AddressUsecase {
	return &AddressUsecase{repo: repo, cache: cache}
}

// AddressRepo 用户地址表。
type AddressRepo interface {
	// FindByID 对应 `UserAddress::where(['user_id'=>$uid,'id'=>$id])->field($fields)->find()`
	FindByID(ctx context.Context, userID uint32, id int64) (*AddressRow, bool, error)
	// FindDefault 对应 `UserAddress::where(['user_id'=>$uid,'is_default'=>1])->field($fields)->find()`
	FindDefault(ctx context.Context, userID uint32) (*AddressRow, bool, error)
	// FindFirst 对应 `UserAddress::where(['user_id'=>$uid])->field($fields)->order('id asc')->find()`
	FindFirst(ctx context.Context, userID uint32) (*AddressRow, bool, error)
}

// AddressRow 对应 field('id,is_default,address')。
//
// ⚠️ 原文的 field 只取这**三列**，其余列（contact_user / contact_phone / fettle）
//
//	即使表里有也不返回。
type AddressRow struct {
	ID        int64
	IsDefault int32
	Address   string
}

// ErrAddressNotFound 对应 `self::setError('未找到地址记录')`。
var ErrAddressNotFound = errors.New("未找到地址记录")

// AddressDefaultCacheTTL 对应 `RedisLockService::set($cacheKey, json_encode($ret), 5)`。
const AddressDefaultCacheTTL = 5 * time.Second

// AddressDefaultCacheKey 对应 `"purchase:addressDefault:$userId"`。
//
// ⚠️⚠️ 键名带 `purchase` 前缀，但这是**用户地址**，和采购毫无关系 ——
// 又一个原项目的命名遗留（和 NoticeLogic::getNew 的 `purchase:getNew` 是同一类）。
// 照搬，不要"顺手改对"，否则与 PHP 的缓存不互通。
//
// ⚠️ 走 RedisLockService（裸 phpredis、无前缀）-> 与 PHP **共用同一把键**。
func AddressDefaultCacheKey(userID uint32) string {
	return "purchase:addressDefault:" + itoa(int64(userID))
}

// AddressDefault 逐字对应 AddressLogic::addressDefault($params, $userId)。
//
// 原文：
//
//	$cacheKey = "purchase:addressDefault:$userId";
//	$ret = RedisLockService::get($cacheKey);
//	if ($ret) { return json_decode($ret, true); }
//	$where = ['user_id' => $userId];
//	$fields = "id,is_default,address";
//	if (isset($params['id']) && $params['id'] > 0) {
//	    $where['id'] = $params['id'];
//	    $address = UserAddress::where($where)->field($fields)->find();
//	} else {
//	    $where['is_default'] = 1;
//	    $address = UserAddress::where($where)->field($fields)->find();
//	    if (empty($address)) {
//	        $address = UserAddress::where(['user_id'=>$userId])->field($fields)->order('id asc')->find();
//	    }
//	}
//	if (empty($address)) { self::setError('未找到地址记录'); return false; }
//	$ret = $address->toArray();
//	RedisLockService::set($cacheKey, json_encode($ret), 5);
//	return $ret;
//
// ⚠️ 三分支的**顺序不能变**：带 id 走精准查询；否则先查默认地址，
//
//	默认地址不存在才退化成"第一条"。
func (uc *AddressUsecase) AddressDefault(ctx context.Context, userID uint32, id int64) (*AddressRow, error) {
	key := AddressDefaultCacheKey(userID)
	if v, err := uc.cache.Get(ctx, key); err == nil && v != nil {
		if s, ok := v.(string); ok && s != "" {
			var row AddressRow
			if json.Unmarshal([]byte(s), &row) == nil {
				return &row, nil
			}
		}
	}

	var (
		row   *AddressRow
		found bool
		err   error
	)
	if id > 0 { // `isset($params['id']) && $params['id'] > 0`
		row, found, err = uc.repo.FindByID(ctx, userID, id)
	} else {
		row, found, err = uc.repo.FindDefault(ctx, userID)
		if err == nil && !found {
			// 默认地址不存在，取用户第一条地址
			row, found, err = uc.repo.FindFirst(ctx, userID)
		}
	}
	if err != nil {
		return nil, err
	}
	if !found || row == nil {
		return nil, ErrAddressNotFound
	}

	// 写缓存（失败不影响返回，原文也没接返回值）
	//
	// ⚠️⚠️ 必须**手工拼 snake_case 的键名**，不能直接 Marshal(row)。
	//    这个缓存键是**与 PHP 共用**的（RedisLockService，无前缀），
	//    PHP 存的是 `json_encode($address->toArray())` = {"id":..,"is_default":..,"address":..}。
	//    直接 Marshal 结构体会输出 Go 的字段名 {"ID":..,"IsDefault":..,"Address":..}，
	//    **PHP 读到就取不到字段** —— 这是接口级的数据不一致，而且只在跨语言读缓存时才暴露。
	//
	//    实测踩过：第一版就是直接 Marshal(row)，缓存里出现了 "ID"/"IsDefault"/"Address"。
	if b, merr := MarshalNoHTMLEscape(map[string]any{
		"id":         row.ID,
		"is_default": row.IsDefault,
		"address":    row.Address,
	}); merr == nil {
		_ = uc.cache.Set(ctx, key, b, AddressDefaultCacheTTL)
	}
	return row, nil
}
