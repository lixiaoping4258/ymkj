# 待迁侦察：`GET /v1/ticket/amount`（用户钱包余额汇总）

> 静态分析结果，**未实现**。`AmountLogic::queryAmount` 已**全部读完**（行 28-104），
> 但发现一个**配置迁移缺口**（见下），需要先补配置。

## 路由与鉴权

- 路径：**`GET /v1/ticket/amount`**（group `v1/ticket`）
- 控制器：`TicketWalletController::amount`
- **需登录**（`TicketWalletController` 未声明 `$notNeedLogin`）

```php
function amount(): Json
{
    $value = AmountLogic::queryAmount($this->userId);
    return $this->data($value);
}
```

## 业务逻辑（`AmountLogic::queryAmount`，行 28-104）

### ① 缓存（**可与 PHP 共用**）

```php
$cacheKey = "user:queryAmount:$userId";
$ret = RedisLockService::get($cacheKey);
if ($ret) { return json_decode($ret, true); }
...
RedisLockService::set($cacheKey, json_encode($return), 5);   // TTL 5
```

⚠️ `RedisLockService`（裸 phpredis、plain JSON）→ Go 侧用 `biz.RawCache` **直接共用**（清单第 15 条）。

### ② 限制时间（⚠️ **配置缺口**）

```php
$envStart = env('APP.LIMITTIMESTART');
$envEnd   = env('APP.LIMITTIMEEND');
$time = [$envStart, $envEnd];
```

实测 `.env` 里是：

```
LIMITTIMESTART = "09:00:00"
LIMITTIMEEND   = "23:00:00"
```

🔴 **`LimitTimeStart` / `LimitTimeEnd` 目前不在 Kratos 的 `conf.App` 里**
（`internal/conf/conf.proto` 没有这两个字段，`configs/config.local.yaml` 也没有）。

**必须先补配置**：在 `conf.proto` 的 `App` 里加 `limit_time_start` / `limit_time_end`，
在 `config.local.yaml` 里填 `"09:00:00"` / `"23:00:00"`，再 `gen.ps1` + `wire`。
**不要用 `os.Getenv` 兜底** —— Go 服务读的是 YAML，环境变量根本不会设，会静默拿到空值。

### ③ 主查询

```php
$res = UserWallet::where(['user_id' => $userId])
    ->field('amount,type_id,frozen_amount')->select()->toArray();
```

⚠️ 表 `x_user_wallet` 共 7 列（`id,user_id,amount,type_id,create_time,frozen_amount,update_time`），
**只取 3 列**。
⚠️ `x_user_wallet` **没有 delete_time** → 不加软删除过滤。

### ④ 逐行按 type_id 补 wait_amount

```php
foreach ($res as $key => $value) {
    $existType[$value['type_id']] = true;
    switch ($value['type_id']) {
        case 4:
        case 2:
            $res[$key]['wait_amount'] = UserGetTicket::where([
                'user_id' => $userId,
                'state'   => CheckStateEnum::STATE_WAIT,   // 0
                'ticket_type' => $value['type_id'],
            ])->sum('change_amount');
            $res[$key]['limit_time'] = $time;
            break;
        case 1:
        default:
            $res[$key]['wait_amount'] = 0;             // ⚠️ 且**不加 limit_time**
            break;
    }
}
```

**`CheckStateEnum`**（已核实）：

```php
const STATE_WAIT   = 0;   // 待审核
const STATE_PASS   = 1;
const STATE_REFUSE = 2;
const STATE_CANCEL = 3;
```

⚠️ `UserGetTicket` **`extends Model`**（不是 BaseModel），**没有 use SoftDelete**
→ 表 `x_user_get_ticket`，**不加** delete_time 过滤。
⚠️ `sum('change_amount')` 在无匹配行时 ThinkPHP 返回 **0**（不是 null）。

### ⑤ 补齐四个 type_id 的空条目

```php
if (!isset($existType[1])) { $res[] = ['amount'=>0,'type_id'=>1,'frozen_amount'=>0,'wait_amount'=>0]; }
if (!isset($existType[2])) { $res[] = [... , 'limit_time'=>$time]; }
if (!isset($existType[3])) { $res[] = ['amount'=>0,'type_id'=>3,'frozen_amount'=>0,'wait_amount'=>0]; }
if (!isset($existType[4])) { $res[] = [... , 'limit_time'=>$time]; }
```

⚠️⚠️ **只有 2 和 4 补 `limit_time`；1 和 3 不补**（与 ④ 的 switch 一致）。
这看起来不对称，但是原文如此 —— 照搬。

### ⑥ 按 type_id 升序

```php
usort($res, function ($a, $b) { return $a['type_id'] - $b['type_id']; });
```

### ⑦ type_id=2 额外加 `Withdraw_status`

```php
foreach ($res as $key => $value) {
    if ($value['type_id'] == 2) {
        $sync_order_num = UserAccounts::where([
            'user_id' => $userId, "app_id" => 3, 'state' => "ENABLE",
        ])->value("sync_order_num");
        $res[$key]['Withdraw_status'] = $sync_order_num > 0 ? 1 : 0;
    }
}
```

⚠️⚠️ **字段名是大写开头的 `Withdraw_status`** —— 全项目唯一的写法，照搬。
⚠️ `app_id = 3`（大海盛世）与 `state = 'ENABLE'` 都是**硬编码**。
⚠️ 实测 `x_user_accounts` 里 **`sync_order_num` 大量为 NULL**（`app_id=1/2` 全是 NULL，
   `app_id=3 ENABLE` 18 行里 10 行 >0）→ `null > 0` 为 **false** → `Withdraw_status = 0`。
⚠️ `x_user_accounts` **有软删除**（第 41 轮踩过）→ 必须加 `delete_time IS NULL`。

### ⑧ 返回

```php
$return = ['state' => 1, 'msg' => '查询成功', 'list' => $res];
```

⚠️⚠️ **注意：这个接口的 data 里又套了 `state` / `msg`** ——
外层信封已经有一层 `code/show/msg/data`，这里 `data` 内部**再有一个 `state` 和 `msg`**。
**不要与外层混淆**，也不要"顺手"去掉（前端可能读的是里层）。

## 对外形态

```json
{
  "state": 1,
  "msg": "查询成功",
  "list": [
    {"amount":"0.00","type_id":1,"frozen_amount":"0.00","wait_amount":0},
    {"amount":"0.00","type_id":2,"frozen_amount":"0.00","wait_amount":0,
     "limit_time":["09:00:00","23:00:00"],"Withdraw_status":0},
    {"amount":"0.00","type_id":3,"frozen_amount":"0.00","wait_amount":0},
    {"amount":"0.00","type_id":4,"frozen_amount":"0.00","wait_amount":0,
     "limit_time":["09:00:00","23:00:00"]}
  ]
}
```

- `amount` / `frozen_amount` 是 **decimal(10,2)** → PHP 输出**字符串**
- `type_id` / `wait_amount` / `Withdraw_status` 是**数字**
- 缺条目的补零行 `amount` 是 **int 0**（PHP 字面量 `0`），**不是 `"0.00"`** ——
  所以同一批数据里 `amount` 的类型可能**既出现字符串又出现数字**，照搬。

## 实现要点清单

| # | 项 |
|---|---|
| 1 | 🔴 **先补 `conf.App` 的 `limit_time_start` / `limit_time_end`**（proto + yaml + gen + wire） |
| 2 | 缓存 `user:queryAmount:{userId}` 用 `biz.RawCache`（与 PHP 共用），TTL 5 |
| 3 | `x_user_wallet` 只取 3 列；无软删除 |
| 4 | `x_user_get_ticket` 无软删除；`sum()` 无匹配返回 0 |
| 5 | **补零时只有 type_id 2/4 带 `limit_time`** |
| 6 | 按 `type_id` 升序 |
| 7 | type_id=2 加**大写 W** 的 `Withdraw_status`；`x_user_accounts` 有软删除 |
| 8 | data 内部再套 `{state,msg,list}`；补零行的 `amount` 是 int 0（类型不统一） |

## 为什么这轮没实现

**第 1 条是硬前置**：`LimitTimeStart/End` 不在 conf 里，必须先改 `conf.proto` +
`config.local.yaml` + 重新 `gen` + `wire`。这是配置层的工作，不是 handler 层。

而且本次会话上下文预算已接近耗尽 —— 在预算耗尽时同时改配置层与业务层，
容易产出难以验证的结果。**先把源码读全并整理成清单**，下一次接手可直接照做。
