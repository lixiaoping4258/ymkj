# 待迁侦察：`POST /v1/user/address/create`（新增地址）

> 静态分析结果，**未实现**。数据与逐行说明都在这里，接手的人不必重读源码。

## 路由与鉴权

- 路径：**`POST /v1/user/address/create`**（group `/v1/user`）
- 控制器：`app/api/controller/v1/user/AddressController.php::create`
- **需登录**（`AddressController` 未声明 `$notNeedLogin`）

## 控制器

```php
function create(): Json
{
    $params = (new UserAddressCreateValidate())->post()->goCheck();

    $result = AddressLogic::create(
        $this->userId,
        $params['flag'],
        $params['user']    ?? '',
        $params['phone']   ?? '',
        $params['address'] ?? '',
        $params['fettle']  ?? '',
        intval($params['default']) == 1
    );
    if ($result) { return $this->success('添加成功'); }   // ⚠️ msg = "添加成功"
    else { return $this->fail(AddressLogic::getError()); }
}
```

## 业务逻辑（`AddressLogic::create`，行 28-62）

```php
static function create(int $userId, string $appFlag, string $concatUser = '',
                       string $concatPhone = '', string $address = '',
                       string $fettle = '', bool $default = false): bool
{
    // ① 按 flag 查发行平台
    $openApp = OpenAppLogic::findAppByFlag($appFlag);
    if (!$openApp) { self::setError('未找到发行平台'); return false; }

    $data = [
        'id'            => Id::gen(),          // ⚠️ 共享的 Redis ID 生成器（已迁）
        'user_id'       => $userId,
        'app_id'        => $openApp['id'],
        'contact_user'  => $concatUser  ?? '',
        'contact_phone' => $concatPhone ?? '',
        'address'       => $address     ?? '',
        'fettle'        => $fettle      ?? '',
        'is_default'    => $default ? 1 : 0,
    ];

    Db::startTrans();                          // ② 事务
    try {
        if ($default) {
            // ③ 设为默认前，先把该用户已有的默认地址全部清掉
            UserAddress::where(['user_id' => $userId, 'is_default' => 1])
                ->save(['is_default' => 0]);
        }
        UserAddress::create($data);
        Db::commit();
        return true;
    } catch (\Exception $e) {
        Db::rollback();
        self::setError($e->getMessage());      // ⚠️ 异常信息会直接展示给用户
        return false;
    }
}
```

### `OpenAppLogic::findAppByFlag($flag)`

```php
$condition = ['flag' => $flag, 'type' => 2, 'state' => 1];
$res = App::where($condition)->findOrEmpty();
if ($res->isEmpty()) { return false; }
return $res->toArray();
```

- **`type = 2` 与 `state = 1` 都是硬编码的**
- 实测 `x_app` 里 `type=2` 且 `state=1` 的只有一行：`id=1234554321, name=元梦典藏, flag=ymdc`

## 校验（`UserAddressCreateValidate`）

**规则**（部分已确认）：

```php
'fettle'  => 'max:100',
'default' => 'require|in:1,0',
// 以下三条的规则在文件上半部分，**尚未读到**，但从 message 可反推：
'flag'    => 'require',              // 推断
'user'    => 'max:20',               // 推断（message 里有 user.max）
'phone'   => 'max:11|mobile',        // 推断（message 里有 phone.max 和 phone.mobile）
'address' => 'require|max:100',      // 推断（message 里有 address.require / address.max）
```

**文案**（已确认）：

```php
'flag.require'    => '请选择地址类型',
'user.max'        => '收货人名称不能超过20个字符',
'phone.max'       => '手机号码不能超过11个字符',
'phone.mobile'    => '手机号码格式不正确',
'address.require' => '请填写收货地址',
'address.max'     => '收货地址不能超过100个字符',
'fettle.max'      => '收货地址详情不能超过100个字符',
'default.require' => '请选择是否为默认地址',
'default.in'      => '请选择是否为默认地址',
```

### ⚠️ 实现前必须先补的两件事

| # | 项 |
|---|---|
| 1 | **`UserAddressCreateValidate` 上半部分的规则**（`flag`/`user`/`phone`/`address` 四条的确切写法）—— 现在只有从 message 反推的猜测 |
| 2 | **`mobile` 规则的正则**：ThinkPHP 的 `mobile` 验证器用的是固定正则；Go 侧必须用**同一条正则**，否则"哪些手机号算合法"会不一致。要读 `vendor/topthink/framework/src/think/validate/Validate.php` 或 `Check` 类里的实现 |

## 实现要点清单

1. **`Id::gen()`** —— 用已迁的 `biz.IDGenerator`（与 PHP 共用 Redis 键，见 `data/idgen.go`）
2. **事务**：清旧默认 + 插入必须同一事务
3. **`app_id` 来自按 flag 查到的 `x_app.id`**，不是请求里传的
4. **`state=1` 的两个默认值**：`is_default` 为 0 或 1（`intval($params['default']) == 1`）
5. **响应**：`success('添加成功')` → `msg="添加成功"`，`data=[]`（用 `RawBody()` 机制，与 `DeleteAddress` 同）
6. **异常信息直接回显**：`setError($e->getMessage())` —— 原实现会把数据库异常信息展示给用户

## 为什么这轮没实现

两个待补项（校验规则全文 + `mobile` 正则）都需要再读源码；且这是**写操作**，
而本次会话上下文预算已接近耗尽。

**更重要的一条纪律**：上一轮我在验证 `address/delete` 时**误用真实数据**做破坏性测试，
造成一次数据事故（已恢复）。在预算耗尽的状态下继续写**写操作**接口，
风险与收益不成比例 —— 所以先把已读到的信息固定下来。
