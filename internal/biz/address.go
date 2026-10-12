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

/* ------------------------------------------------- 地址列表 */

// AddressListUsecase 对应 app/api/lists/users/AddressLists.php。
type AddressListUsecase struct {
	repo AddressListRepo
}

func NewAddressListUsecase(repo AddressListRepo) *AddressListUsecase {
	return &AddressListUsecase{repo: repo}
}

// AddressListRepo 地址列表（含 x_app 的 INNER JOIN）。
type AddressListRepo interface {
	ListAddresses(ctx context.Context, userID uint32, offset, limit int) ([]map[string]any, error)
	CountAddresses(ctx context.Context, userID uint32) (int64, error)
}

// List 对应 dataLists(new AddressLists())。
//
// 原实现：
//
//	private function queryWhere() { return ['UserAddress.user_id' => $this->userId]; }
//
//	public function lists(): array {
//	    return UserAddress::hasWhere('app', $this->queryWhere(),
//	        'App.name platform_name,App.flag,UserAddress.id,UserAddress.contact_user,
//	         UserAddress.contact_phone,UserAddress.address,UserAddress.fettle,UserAddress.is_default')
//	        ->limit($this->limitOffset, $this->limitLength)->select()->toArray();
//	}
//	public function count(): int {
//	    return UserAddress::hasWhere('app', $this->queryWhere())->count();
//	}
//
// ⚠️⚠️ **`hasWhere('app', ...)` 生成的是 INNER JOIN**（不是 LEFT JOIN）：
//
//	FROM x_user_address UserAddress
//	INNER JOIN x_app App ON App.id = UserAddress.app_id
//	WHERE UserAddress.user_id = ?
//
// 关联定义见 UserAddress::app()：`hasOne(App::class, 'id', 'app_id')`。
//
// **后果**：`app_id` 在 `x_app` 里不存在的地址**不会出现在列表里**，
// 而且 `count()` 用的是同一个 INNER JOIN -> 分页总数也不含它们。
// 实测当前所有地址的 app_id（1234554321）都能匹配上，所以暂时看不出差异；
// 但迁移时不能用 LEFT JOIN —— 那会让"孤儿地址"突然出现，总数也对不上。
//
// ⚠️ list 与 count 必须用**同一套 JOIN 条件**，否则分页会错位。
func (uc *AddressListUsecase) List(ctx context.Context, userID uint32, p PageParams) (*ListData, error) {
	count, err := uc.repo.CountAddresses(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.ListAddresses(ctx, userID, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}

// AddressListSelect 逐字对应原文的字段列表（8 个）。
//
// ⚠️ 顺序即原文顺序：App.name 别名成 platform_name，App.flag 保持 flag，
//
//	其余是 UserAddress 的列（ThinkPHP 会把表别名去掉，键就是列名）。
const AddressListSelect = "app.name AS platform_name, app.flag, ua.id, " +
	"ua.contact_user, ua.contact_phone, ua.address, ua.fettle, ua.is_default"
