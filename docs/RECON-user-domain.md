# 待迁侦察：`user.php`（21 条，已迁 1 条）

> 静态分析结果，**未实现**。目的是让接手的人知道**从哪条开始、哪条是坑**。

## 路由清单（`app/api/route/user.php`，group `/v1/user`）

| 方法 | 路径 | 控制器::方法 | 复杂度 |
|---|---|---|---|
| GET | `/v1/user/info` | `UserController::info` | ✅ **已迁** |
| GET | `/v1/user/center` | `UserController::center` | 🔴 **复杂**（见下） |
| GET | `/v1/user/real/info` | `UserController::getRealInfo` | 🟡 中（实名信息） |
| GET | `/v1/user/unregisr/check` | `UserController::unregisterCheck` | 🟡 中 |
| GET | `/v1/user/address` | `AddressController::index` | 🟢 列表（分页设施已就绪） |
| GET | `/v1/user/address/default` | `AddressController::addressDefault` | 🟢 简单 |
| GET | `/v1/user/address/delete` | `AddressController::delete` | ⚠️ GET 但**写操作** |
| GET | `/v1/user/message` | `MessageController::index` | 🟡 列表 + 用户态 |
| GET | `/v1/user/message/detail` | `MessageController::detail` | 🟡 中 |
| GET | `/v1/user/notice/getNew` | `NoticeController::getNew` | 🟢 简单 |
| POST | `/v1/user/setInfo` | `UserController::setInfo` | 🟡 写 |
| POST | `/v1/user/changePassword` | `UserController::changePassword` | 🟡 写 |
| POST | `/v1/user/bindMobile` | `UserController::bindMobile` | 🟡 写 + 短信 |
| POST | `/v1/user/getMobileByMnp` | `UserController::getMobileByMnp` | 🔴 微信 |
| POST | `/v1/user/real/verify` | `UserController::real` | 🔴 实名认证（外部） |
| POST | `/v1/user/unregister` | `UserController::unregister` | 🔴 注销（多表 + 事务） |
| POST | `/v1/user/resetPassword` | `UserController::resetPassword` | **免登录**（`$notNeedLogin = ['resetPassword']`） |
| POST | `/v1/user/address/create` | `AddressController::create` | 🟢 写 |
| POST | `/v1/user/address/update` | `AddressController::update` | 🟢 写 |
| POST | `/v1/user/message/setAllRead` | `MessageController::setAllRead` | 🟢 写 |
| POST | `/v1/user/message/setOneRead` | `MessageController::setOneRead` | 🟢 写 |

**`UserController::$notNeedLogin = ['resetPassword']`** —— 21 条里**只有 `resetPassword` 免登录**，
其余全部需登录。（`AddressController` / `MessageController` / `NoticeController` 的 `$notNeedLogin` 需各自核对。）

## 🔴 `UserLogic::center`（`app/api/logic/UserLogic.php:70`）—— 为什么它复杂

已读到的部分（方法还没结束）：

```php
public static function center(array $userInfo): array
{
    $uid = $userInfo['user_id'];
    $cacheKey = "user:center:$uid";
    $ret = RedisLockService::get($cacheKey);      // ⚠️ 裸 redis，与 PHP 共用键
    if ($ret) { return json_decode($ret, true); }

    $user = User::where(['id'=>$uid])
        ->field('id,sn,sex,nickname,real_name,avatar,mobile,create_time,password,user_money')
        ->findOrEmpty();
    if (!$user->isEmpty()) {
        // 仅微信小程序/公众号终端才加的字段
        if (in_array($userInfo['terminal'], [WECHAT_MMP, WECHAT_OA])) {
            $auth = UserAuth::where(['user_id'=>$uid,'terminal'=>$userInfo['terminal']])->find();
            $user['is_auth'] = $auth ? YES : NO;
        }
        $user['has_password'] = !empty($user['password']);
        $user->hidden(['password']);              // password 只用于算 has_password，不对外
        $info = $user->toArray();

        $userMoney = $info['user_money'];
        unset($info['user_money']);               // ⚠️ user_money 被**移出 info**
    } else {
        $info = []; $userMoney = 0;
    }

    $centerData = [
        'info'       => $info,
        'market'     => [],
        'wallet'     => ['balance' => $userMoney, 'income' => 0],
        'collection' => ['total' => 0],
    ];

    // 查询订单：秒转订单待付款数量
    $condition = ['user_id'=>$uid, 'state'=>'WAIT_PAY', 'pay_state'=>['WAIT']];
    // ... 方法在此之后还有若干查询，**未读完**
```

**已知的坑**：

1. **`user_money` 被从 `info` 里移出**，改放到 `wallet.balance` —— 别把两个都输出
2. **`is_auth` 是条件字段**：只有 `terminal ∈ {1,2}`（微信小程序/公众号）时才存在
3. **`password` 只为算 `has_password`**，`hidden(['password'])` 后不对外
4. **`market` 恒为 `[]`、`collection.total` 恒为 0** —— 源码里就是写死的，不是查询结果
5. **有裸 redis 缓存**（`user:center:$uid`，`RedisLockService` → 与 PHP **共用键**）
6. `market` / `collection` 里的 `[]` / `0` 是**字面量**，说明这部分是半成品待实现

**未读完的部分**：`$condition` 之后还有秒转订单查询（约 50+ 行）。

## 建议的接手顺序

**从最简单、无写操作、无外部依赖的开始**：

1. **`GET /v1/user/notice/getNew`** —— 🟢 最简单（`NoticeController::getNew`），且**免登录**（`$notNeedLogin = ['getNew']`）
2. **`GET /v1/user/address/default`** / **`address`（列表）** —— 分页设施已就绪
3. **`GET /v1/user/real/info`** —— 中等
4. **`GET /v1/user/message`（列表）** —— 复用分页设施
5. `POST` 系列（写操作）—— 需要先理清幂等/事务
6. **`GET /v1/user/center`** —— 最后做，要先把剩下的 50+ 行读完

## ⚠️ 一个容易标反的地方

**`GET /v1/user/address/delete` 是 GET 但它是写操作**（删地址）。
把它当只读接口处理会漏掉写权限、事务与幂等考虑。
（`Route::get('address/delete', ...)` —— 用 GET 做删除是原项目的设计。）

## 为什么这轮没实现

`UserLogic::center` 已读部分就有 6 个坑（条件字段、字段移出、裸 redis 缓存、
写死的空值），且方法**还没读完**（约 50+ 行未读）。
本次会话上下文预算已接近耗尽，硬写会产出无法验证的实现。
先把读到的信息固定下来。
