# 待迁侦察：`GET /v1/market/purchase/detail`（兑换单详情）

> 静态分析结果，**未实现**。`MarketListPurchaseLogic::getSaleDetail` 已**全部读完**，
> 信息足够，接手可直接照此实现。

## 路由与鉴权

- 路径：**`GET /v1/market/purchase/detail?id=N`**（group `v1/market`）
- 控制器：`PurchaseController::detail`（`$notNeedLogin = ['index']` → **需登录**）

```php
public function detail(): Json
{
    $params = (new PurchaseDetailValidate())->get()->goCheck();
    $data = MarketListPurchaseLogic::getSaleDetail($params['id'], $this->userId);
    if ($data === false) { return $this->fail('未找到记录信息'); }
    return $this->data($data);
}
```

**校验**（`PurchaseDetailValidate`）：

```php
'id' => 'require|integer|gt:0',
'id.require' => '详情ID不能为空',
'id.integer' => '详情ID不正确',
'id.gt'      => '详情ID必须大于0',
```

⚠️ 与 `SaleDetailValidate`（`require|number`）**不同** —— 这里有 `gt:0`。清单第 13 条。

## 业务逻辑（`MarketListPurchaseLogic::getSaleDetail`，行 69-161）

### ① 缓存（**可与 PHP 共用**）

```php
$cacheKey = "purchase:getSaleDetail:$id";
$ret = RedisLockService::get($cacheKey);
if ($ret) { return json_decode($ret, true); }
...
RedisLockService::set($cacheKey, json_encode($ret), 5);   // TTL 5
```

⚠️ 走 **`RedisLockService`**（裸 phpredis、无前缀、plain JSON）→ Go 侧可用
`biz.RawCache`（`data.NewRawRedisCache`）**直接读写并与 PHP 共用**。
（清单第 15 条：TP 的 `cache()` 才是 PHP serialize 读不了的。）

⚠️ 命名上又是 `purchase:` 前缀（与 `purchase:getNew`、`purchase:addressDefault:` 同类）。

### ② 主查询（两 JOIN，**17 个字段**）

```php
$purchaseInfo = MarketPurchase::alias('market_purchase')
    ->where(['market_purchase.id' => $id,
             'market_purchase.state' => ['OFFLINE', 'WANTED', 'COMPLETE']])
    ->join('app_archive', 'market_purchase.archive_id=app_archive.id')
    ->join('app',         'market_purchase.app_id=app.id')
    ->field([
        'market_purchase.id',
        'market_purchase.pay_way',
        'market_purchase.user_id',
        'market_purchase.app_id',
        'market_purchase.user_address_id',
        //'market_purchase.sn',              // 🔴 被注释掉了，输出里没有 sn
        'market_purchase.amount',
        'market_purchase.receive_amount',
        'market_purchase.unit_price',
        'market_purchase.state',
        'market_purchase.content',
        'market_purchase.public_time',
        'market_purchase.end_time',
        'app_archive.collection_id',
        'app_archive.name',
        'app_archive.images',
        'app.name platform_name',
        'app.id platform_id',
    ])->findOrEmpty();
if ($purchaseInfo->isEmpty()) { return false; }        // -> fail('未找到记录信息')
```

⚠️ **`state IN ('OFFLINE','WANTED','COMPLETE')`** —— 与列表接口一致；
`EXPIRE` 的兑换单**查不到**。
⚠️ **`sn` 被注释掉**，所以输出里**没有 `sn`**。
⚠️ `images`：`!empty` 才 json_decode（**没有 else**，空值保持原值）。
⚠️ `x_market_purchase` **有软删除**（第 6 轮踩过）→ 必须加 `delete_time IS NULL`。

### ③ available_amount（**已有可复用实现**）

```php
$stockNum = PurchaseOrderStockLogic::getNum($purchaseInfo['id']);
$purchaseInfo['available_amount'] =
    $stockNum == null ? bcsub($purchaseInfo['amount'], $purchaseInfo['receive_amount']) : $stockNum;
```

✅ **已实现可直接复用**：`biz.PurchaseStateRepo.StockNum(ctx, id) (int64, bool, error)`
以及 `internal/biz/purchase_state.go` 里对 `$stockNum == null` 语义的完整说明
（**PHP 里 `0 == null` 为真**，所以"库存为 0"也走 bcsub 兜底 —— 不要简化成 `exists ? num : fallback`）。

### ④ has_order

```php
$ret["has_order"] = 0;
$PurchaseOrders = PurchaseOrders::where([
    "purchase_id" => $id,
    'state' => [ORDER_CREATE, ORDER_PROCESS, ORDER_COMPLETE],   // 'CREATE','PROCESS','COMPLETE'
])->find();
if ($PurchaseOrders) { $ret["has_order"] = 1; }
```

### ⑤ able_exchange

```php
$checkMarketPurchase = MarketPurchase::where([
    "user_id" => $userId, "state" => STATE_WANTED,      // 'WANTED'
])->findOrEmpty();
$ret["able_exchange"] = !$checkMarketPurchase->isEmpty() ? 0 : 1;
```

⚠️ 注意语义：**该用户自己有 WANTED 单时 able_exchange=0**（不能兑换）。
⚠️ 这是"当前用户"，不是详情里的买家。

### ⑥ saler（与 `SalesDetail` 同形）

```php
$salerInfo = User::where(['id' => $purchaseInfo['user_id']])->field('nickname,avatar')->findOrEmpty();
if (!$salerInfo->isEmpty()) {
    $ret['saler'] = $salerInfo->toArray();
    $ret['saler']['sale_count']       = MarketSales::where(['user_id'=>..,'state'=>['ON_SALE']])->count();
    $ret['saler']['done_count']       = MarketSales::where(['user_id'=>..,'state'=>['SELL_OUT']])->count();
    $ret['saler']['collection_count'] = 0;          // ⚠️ 硬编码
}
```

⚠️ `avatar` 经 `User::getAvatarAttr` 拼域名。
⚠️ 查不到用户时 `saler` 保持 **空数组 []**（初值），且不补后三个键。

### ⑦ trade_notice

```php
$notice = WebSettingLogic::getTradeNotice();     // ['content'=>ConfigService::get('trade','notice')]
$ret['trade_notice'] = $notice['content'];
```

⚠️ **无默认值 → 可能 null**。（已实现：`ConfigUsecase.GetDefault(ctx,"trade","notice",nil)`。）

### ⑧ buyer_info（嵌套对象，8 个键）

```php
$appName = App::where('id', $purchaseInfo['app_id'])->value('name');
$buyer = User::where('id', $purchaseInfo['user_id'])->findOrEmpty();
$buyerInfo = $buyer->isEmpty() ? [] : $buyer->toArray();
$buyAddress = UserAddress::where('id', $purchaseInfo['user_address_id'])->findOrEmpty();
$buyAddress = $buyAddress->isEmpty() ? [] : $buyAddress->toArray();
$ret['buyer_info'] = [
    'nickname'        => $buyerInfo['nickname'] ?? '',
    'app_name'        => $appName,              // ⚠️ 可能是 null（App::value 查不到）
    'phone'           => $buyerInfo['mobile'] ?? '',
    'is_have_address' => !empty($buyAddress),   // bool
    'contact_user'    => $buyAddress['contact_user'] ?? '',
    'contact_phone'   => $buyAddress['contact_phone'] ?? '',
    'address'         => $buyAddress['address'] ?? '',
    'fettle'          => $buyAddress['fettle'] ?? '',
];
```

⚠️ **注意这里查了两次 User**（⑥ 的 saler 也用 `$purchaseInfo['user_id']`）——
   是同一个人（**买家**），但 ⑥ 叫 `saler`。**原项目的命名容易误导**：这个"卖家"信息
   实际是**买家自己**（因为 `user_id` 是下单方）。照搬，不改字段名。

## 对外形态

```json
{
  "collection": { 16 个字段（17 减去被注释的 sn）+ available_amount },
  "saler": { nickname, avatar, sale_count, done_count, collection_count } | {},
  "has_order": 0 | 1,
  "able_exchange": 0 | 1,
  "trade_notice": "..." | null,
  "buyer_info": { nickname, app_name, phone, is_have_address,
                  contact_user, contact_phone, address, fettle }
}
```

**6 个顶层键**（`collection` / `saler` / `has_order` / `able_exchange` / `trade_notice` / `buyer_info`）。

## 实现要点清单

| # | 项 |
|---|---|
| 1 | 缓存用 `biz.RawCache`（**与 PHP 共用**），键 `purchase:getSaleDetail:{id}`，TTL 5 |
| 2 | 主查询两 JOIN、**17 字段**（`sn` 被注释 → 实际 16）、`state IN ('OFFLINE','WANTED','COMPLETE')` |
| 3 | `x_market_purchase` **有软删除** → 加 `delete_time IS NULL` |
| 4 | `available_amount` 复用 `PurchaseStateRepo.StockNum` + `PurchaseStateRow.ToMap` 的语义 |
| 5 | `has_order` / `able_exchange` 是两个独立的查询 |
| 6 | `saler` 的三个键里 `collection_count` **硬编码 0** |
| 7 | `buyer_info.app_name` 来自 `App::value('name')`，**查不到是 null** |
| 8 | datetime 列（`public_time` / `end_time`）必须格式化（清单第 1 条） |
| 9 | 响应是**对象**（6 个键），不是数组 —— 用 `RawData` 手工拼 |

## 为什么这轮没实现

方法约 105 行、6 个子部分，且需要一个新的 17 字段行类型
（现有的 `PurchaseStateRow` 是**列表用的另一组字段**，不能复用）。
估算 Go 侧约 250 行。**本次会话上下文预算已接近耗尽**，
在预算耗尽时写这个规模的实现，风险是产出无法验证的代码。

**已把源码读全并整理成上面的清单** —— 下一次接手不需要重读 PHP。
'@
