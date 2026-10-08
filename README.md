# xTravel → Kratos 重构工程

把 `D:\work\phpstudy_pro\WWW\xTravel`（ThinkPHP 8，**754 个 PHP 文件 / 64,588 行**）
用 Kratos v2 重构。本仓库只在 `develop` 分支改动，不推远端。

---

## 一、当前进度：Stage 1 已完成并**连真实数据库验证通过**

重构不是一次性推倒重写 —— 那是 6 万行的工程量，硬写只会产出垃圾。
按可验证的垂直切片推进，每个切片都要求「协议兼容 + 连真库跑通」。

| 阶段 | 内容 | 状态 |
|---|---|---|
| **Stage 1** | Kratos 骨架、基础设施、协议兼容层、`v1/common` 垂直切片 | ✅ 完成并验证 |
| **Stage 2** | 鉴权中间件、token 生命周期、`v1/user/info`、单元测试 | ✅ 完成并验证 |
| **Stage 3** | `market` 域第 1 批：`pay_way`、`check/exchange` + 契约修正 | ✅ 完成并验证 |
| **Stage 3** | 白名单中间件 + 分页基础设施（77 个列表类的公共前置件） | ✅ 完成并验证 |
| **Stage 3** | `market` 域第 2 批：`purchase` 列表接口 + **免登录白名单审计修正** | ✅ 完成并验证 |
| **Stage 3** | `market` 域第 3 批：`sales/categories`、`sales` 列表 | ✅ 完成并验证 |
| **Stage 3** | 第 4 批：`stock/lookall`、`purchase/info`、`sales/info` | ✅ 完成并验证 |
| **Stage 3** | 第 5 批：费率计算内核（`calculateFeeRate`/`calcFeeTtl`） | ✅ 完成并验证 |
| **Stage 3** | 第 6 批：`purchase/showTotalAmount`（接上费率解析） | ✅ 完成并验证 |
| **Stage 3** | 第 7 批：兑换单列表数据层 + `purchase/out`、`purchase/on` | ✅ 完成并验证 |
| **Stage 3** | 第 8 批：缓存层接入 + **修掉列表接口忽略 query 参数的 bug** | ✅ 完成并验证 |
| Stage 3+ | `market` 域其余 33 条（含全部写操作） | 待做 |
| Stage 4 | `wallet` / `payment` / `ticket` / `whitelist` | 待做 |
| Stage 5 | `adminapi`（61 控制器，最大一块） | 待做 |
| Stage 6 | `open` / `third` / 队列消费者 | 待做 |

已迁接口：`v1/common/config`、`v1/common/trade/config`、`v1/user/info`、`v1/market/pay_way`、
`v1/market/check/exchange`、`v1/market/purchase`、`v1/market/sales/categories`、`v1/market/sales`、
`v1/market/stock/lookall`、`v1/market/purchase/info`、`v1/market/sales/info`、
`v1/market/purchase/showTotalAmount`、`v1/market/purchase/out`、`v1/market/purchase/on`
（共 11 条 market 路由 + 3 条非 market）。

### Stage 1 验证记录（真实数据库 `xmarket_test`）

```
GET /v1/common/config
HTTP 200
{"code":1,"show":0,"msg":"","data":{
  "outFee": 6.66,
  "tradeSwitch": {
    "tradeSwitch": 1, "isOpen": 0,
    "todayType": "workday", "todayTypeStr": "工作日",
    "periods": [{"start":"09:30","end":"11:30"}],
    "tips": "兑换时段为上午09:30-11:30，下午13:30-15:30；周末及节假日暂停兑换！"
  },
  "buyOrSaleConfig": {"buy":{...},"sale":{...}}
}}

GET /v1/common/trade/config
HTTP 200
{"code":1,"show":0,"msg":"","data":{"switch":0,"limitTime":1}}
```

信封 `code/show/msg/data` 与原 PHP **逐字段一致**；数据是真实读库结果。

### Stage 2 验证记录（真实 token，四种情况）

```
GET /v1/common/config        无 token   HTTP 200  code=1     ← 公共接口，白名单放行
GET /v1/user/info            无 token   HTTP 200  code=-403 show=0 "请求认证信息有误，请重新登录"
GET /v1/user/info            错误 token HTTP 200  code=-403 show=0 "登录超时，请重新登录"
GET /v1/user/info            真实 token HTTP 200  code=1
    id=61089517  nickname=丝雪  mobile=18384123993
    userMoney="0.00"(字符串)  hasPassword=true  hasOptPwd=false  isReal=false
```

重点看 `userMoney`：它是 `decimal(10,2)`，ThinkPHP 输出的是**字符串** `"0.00"` 而不是数字。
Go 版用 string 承载，类型完全一致 —— 这类字段类型不一致最容易让前端静默出错。

单元测试（`internal/biz`）：

```
ok  github.com/lixiaoping4258/ymkj/internal/biz
```

覆盖：PHP 真值语义边界、token 缓存回源/降级、过期会话判定、
软删除用户拒绝、续期/登出/签发、token 格式与唯一性。
**这些用例是「Go 有没有真的复刻 PHP」的唯一可回归证据**，
改 `phpval.go` / `token.go` 之前先跑 `go test ./...`。

### Stage 3 验证记录（market 域第 1 批）

```
GET /v1/market/pay_way           无 token  code=-403            ← market 接口需要登录
GET /v1/market/pay_way           有 token  code=1
    data = [{"pay_way":1,"pay_name":"茶交所"},{"pay_way":2,"pay_name":"陶交所"}]
           ^^^^^^^^ 顶层数组，snake_case 键名

GET /v1/market/check/exchange    有 token  code=0 show=1
    msg = "当前不在兑换时间内，暂无法导入数字艺术品"
```

`check/exchange` 这条把三件事一次验穿：**2 秒防重复点击锁**（与 PHP 共用 Redis 键）
→ **交易时段判断**（复用 Stage 1 的 `TradeConfigService`）→ **业务错误转 fail 信封**。
当前时间 16:22 不在 09:30-11:30 内，所以按预期被拦下。

**最重要的验证是与真实 PHP 逐字段比对**：

```
PHP: {"id":61089517,"sn":178056289917980,"create_time":1783678044,
      "user_money":"0.00","opt_pwd":null,...}
GO : {"id":61089517,"sn":178056289917980,"create_time":1783678044,
      "user_money":"0.00","opt_pwd":null,...}

键名完全一致 (15 个)          所有字段类型零差异
```

这次比对**抓出了三个 Stage 2 遗留的静默 bug**（见 4.1 节），
说明"能跑通"和"契约一致"是两件事 —— 冒烟测试断言因此从 22 项加到 47 项。

### Stage 3 第二批：白名单中间件 + 分页基础设施

两块都是**全项目共用的前置件**，所以先做它们再铺接口：

**① 白名单（`biz/whitelist.go` + `server/middleware_whitelist.go`）**

对应 `WhitelistMiddleware` + `WhitelistLogic`。market 域有 8 条路由挂着它。
原项目把限制项绑在路由上（`->append(['whitelist_item' => 'no_collection_trade'])`），
Kratos 没有这个东西，改成 `server/middleware_whitelist.go` 里的一张
operation → 限制项 表。**改 proto 的 service/method 名时必须同步改那里**，
否则白名单会静默失效。

实测数据（`xmarket_test`）：5 个限制项、3 个组、**只有 1 个白名单用户**、
9 条组配置**全是 enabled=0** —— 也就是说**现在白名单不拦截任何人**，
正好是干净的迁移基线。

**② 分页（`biz/lists.go`）**

对应 `BaseDataLists`。**全项目 77 个列表类**（39 adminapi + 29 api + 9 common）
都继承这套模式，所以这层做对了，后面每个列表接口都是照抄。

`PageParams` 的语义坑见 4.2 节。

**③ 列表响应的 `data` 形态**

```
{"lists":[...],"count":N,"page_no":1,"page_size":25,"extend":[]}
```

全是 snake_case，且 `extend` 为空时是 `[]` 不是 `null`。
由 `biz.ListData.ToJSON()` 用 map 组装（不用 struct tag，避免改字段名时悄悄变键名）。

单测覆盖：白名单 9 项（含未知限制项、组停用、细粒度 pay_way/兑换单判断）、
分页 8 项。`go test ./internal/biz/...` 共 **32 项全绿**。

> ⚠️ 白名单中间件目前**还没有挂到任何已迁接口上**（它要拦的 `GET /v1/market/purchase`
> 还没迁），所以它只有单测覆盖、没有端到端验证。绑定表里已经预留了该 operation。

### Stage 3 第三批：`purchase` 列表接口 + 两处免登录审计修正

`GET /v1/market/purchase` 一次用上前面所有基础设施：
**免登录 + 白名单 + 分页 + 交易时段判断 + 20 秒缓存**。

**方法上的关键一步：不再靠读代码推断 SQL，而是让 PHP 直接吐出来。**

ThinkPHP 能在 CLI 里启动：

```php
require __DIR__ . '/vendor/autoload.php';
$app = new \think\App(); $app->initialize();

// 生成 SQL 但不执行 —— 绕开 APP_DEBUG 和 userId 的依赖
echo MarketListPurchase::hasWhere('archive', [], $field)->fetchSql(true)->select();

// 或者直接跑列表类，再用 Db::getLastSql() 拿真实执行的 SQL
$l = new \app\api\lists\market\purchase\PurchaseFaceLists();
$l->lists(); Db::getLastSql();
```

这一步澄清了两个**猜不出来**的事实：

1. `hasWhere()` 生成的是 **INNER JOIN**，不是 EXISTS 子查询
2. `$where` 为空数组时（生产测试用户分支）**整个 WHERE 子句消失**，连 `state=1` 都不过滤

同一次探测还拿到了 5 种排序的 `ORDER BY`、6 个筛选条件的 `WHERE`、最终响应信封、
以及每一行的实际值形态。

**验证**：`internal/data/purchase_integration_test.go` 连真实库跑等价查询，
期望值全部来自上面那次 PHP 实测输出：

```
TestPurchaseFaceRepo_MatchesRealPHP   PASS
TestPurchaseFaceRepo_SortVariants     PASS
```

count=3 与 PHP 一致；11 个字段逐字匹配（含 `issuer_time` 的 `Y-m-d H:i:s` 格式、
`"--"` 替换、`images` 解码成数组）；键集合完全一致 —— 多一个少一个都是静默的契约破坏，
所以断言了键数量并逐个核对键名。

**列表接口的 `data` 出口**：用 `v1.RawData` 承载预先组装好的 JSON。
列表行是任意结构（bigint 主键、decimal 字符串、json 数组、`0 转 --` 后处理），
用 `google.protobuf.Struct` 会把数字转成 float64 而**丢掉 bigint 精度**
（`id` 是 178056289901818 这个量级）。

### 4.3 免登录白名单必须逐个对着 PHP 核，不能凭接口名猜

原项目在控制器上声明 `$notNeedLogin`，而**同一个控制器里不同方法的公开性可能不同**：

| 控制器 | `$notNeedLogin` | 结论 |
|---|---|---|
| `v1/common/ConfigController` | `['index']` | `GetConfig` **免登录** ✔ |
| `IndexController` | `['test','index','config','policy','decorate']` | `GetTradeConfig` **需登录** ← Stage 1 误标为公开 |
| `v1/user/UserController` | `['resetPassword']` | `GetUserInfo` 需登录 ✔ |
| `v1/market/PurchaseController` | `['index']` | `PurchaseIndex` **免登录**；`CheckExchange`/`GetPayWay` 需登录 ✔ |

**两个方向都踩了：**

- `GetTradeConfig` 被标成公开 —— 该登录的没登录，是**未授权访问**
- `PurchaseIndex` 忘了标公开 —— 该公开的要登录，**前端匿名访问直接 403**

更值得记的是**为什么没被早发现**：Stage 1 的冒烟测试断言
「`trade/config` 无 token 时 `code=1`」，而那个期望值是**从我自己（错误）的实现里抄的**——
等于拿错误当标准去验证错误。**测试写得再全，期望值来源错了就白搭。**

已修正断言，并补了反向断言（该公开的必须公开）。

**规则**：每迁一个接口，先回 PHP 找它所属控制器的 `$notNeedLogin`，
确认这个方法在不在里面，两个方向的错都要防。

### Stage 3 第四批：`sales/categories` + `sales` 列表

`SaleController` 的公开性也逐个核过：
`$notNeedLogin = ['categories']` → **`categories` 免登录，`index` 需要登录**。

两个接口：

| 接口 | 原实现 | 说明 |
|---|---|---|
| `GET /v1/market/sales/categories` | `SaleController::categories` | **硬编码** `[{"title":"全部","key":"all"}]`，不查库；免登录 |
| `GET /v1/market/sales` | `SaleController::index` → `SaleFaceLists` | 需要登录；复用分页基础设施 |

**两个列表的差异（都是实测出来的，不是推断）**：

| | `PurchaseFaceLists` | `SaleFaceLists` |
|---|---|---|
| `switch($sort)` 有 default 分支 | ✅ 有 → 默认 `purchase_lists DESC` | ❌ 没有 → **不带 sort 时 SQL 里根本没有 ORDER BY** |
| 行后处理 | `purchase_lists` 假值转 `"--"`，并连带两个单价转 `"--"` | 只有 `images` 的 json 解码 |
| 控制器层 | 有交易时段判断 + 20 秒缓存 + 生产测试用户分支 | 只有一句 `return $this->dataLists(...)` |

> 第二个列表的**代码量确实小得多**（没有交易时段、没有缓存），说明分页基础设施起到了作用。
> 但两个 repo 的 WHERE/SELECT 仍有重复 —— 下批再抽公共构造器，届时两个集成测试会兜住回归。

**验证**：`internal/data` 的集成测试新增 `TestSaleFaceRepo_MatchesRealPHP`。
该接口**行级验证做不了**：`x_market_list_sales` 只有 4 行，与 `state=1` 的 archive
join 后是 **0 行**（已直查确认，PHP 跑出来同样是 `count=0 / rows=0`）。
所以断言的是 count 一致、SQL 可跑、6 种排序（含"无 ORDER BY"分支）都安全。
这是**如实说明验证边界**，不是"验过了"。

冒烟测试断言从 58 项加到 **64 项**，全部通过。

### Stage 3 第五批：修一个我自己的软删除 bug + 记录两处待办

**🔴 `MarketPurchase` 的软删除过滤漏了（我 Stage 3 埋的）**

`MarketPurchase` 用了 ThinkPHP 的 SoftDelete，**PHP 侧所有查询都会隐式加
`delete_time IS NULL`，GORM 不会**。我写 `checkExchange` 的计数时漏了这个条件：

```go
// 错的：会把软删除的兑换单也算进来
Where("user_id = ? AND state = ?", userID, state)
```

后果：软删除的 WANTED 兑换单被算进「当前是否存在兑换单」，
导致用户被**错误拦下**（"当前存在兑换单，暂无法导入数字艺术品"）。

**怎么发现的**：读 `PurchaseStateLists` 源码时，第 90 行的注释里贴了一段真实 SQL，
末尾带着 `AND MarketPurchase.delete_time IS NULL`。也就是说这条线索一直明晃晃地
写在原项目的注释里，我却是第四轮才看到。

**实测数据**：`x_market_purchase` 共 750 行，其中 **65 行软删除**。
测试用例取 `user_id=16 + state='OFFLINE'`：全量 95 行、存活 69 行，
漏过滤会得 95。新增 `TestMarketPurchase_SoftDeleteFilterIsApplied` 锁住，
并断言"不能等于全量行数"——这样将来数据变了也不会误判成通过。

注意本表的 `delete_time` 是 **datetime**，而 `x_user` 的是 int unsigned：
判空写法一样，类型不同，模型里别混用。
`PurchaseOrders` 也用了 SoftDelete，迁移订单域时要一并处理。

**📌 待办一：`salesOn` 已实现缓存击穿防护，`salesOut` 没有**

> ✅ **已实现 helper**：`biz/stalecache.go` + `biz/stalecache_test.go`（8 条单测）。
> `StaleCache.Serve()` 逐行对照下面这段 `salesOn` 的控制流。
> ⚠️ 但它**目前还没有被任何接口使用** —— 第一个消费者是 `purchase/on`（`salesOn`），
> 而那个接口依赖的 `PurchaseStateLists` 比较复杂（见待办二），所以分两批做。

`PurchaseController::salesOn` 里是一套完整的 **stale-while-revalidate**：
逻辑过期 5 秒 + 物理 TTL 15 秒 + 重建锁 5 秒。

```php
$payload = ['value'=>..., 'expire_at'=>time()+5];              // 逻辑过期
RedisLockService::set($cacheKey, json_encode($payload), 15);   // 物理 TTL 只兜底
if ($payload && $payload['expire_at'] > time()) return $payload['value'];
$token = tryLock($lockKey, 5000);
if ($token === false) {
    if ($payload) return $payload['value'];   // 没抢到锁 -> 用旧值顶着，绝不排队
    return $this->dataLists(...);             // 冷启动 -> 直查，且不写缓存
}
```

**这正是十几轮前讨论过的「缓存过期 + 很多人同时刷新」问题，原项目里已经实现了。**
而 `salesOut` 用的是简单版（无锁、无逻辑过期），两者不一致 —— 迁移时**逐字保持各自的
行为**，不要"顺手统一"。另外 `salesOn` 里的 `ksort($params)` 注释写着
"固定顺序，否则 `?a=1&b=2` 与 `?b=2&a=1` 是两个键"，与我在
`purchaseIndexCacheKey` 里做的处理一致（而 `index`/`salesOut` 都没做）。

**📌 待办二：`PurchaseStateLists` 比 face list 复杂，迁移要小心**

- 构造时要 `MarketListPurchaseLogic::findSales($id)` 查库，查不到直接抛异常
- `queryWhere` 分 state 两支，用 `end_time > now`、`receive_amount > 0`
- `APP_DEBUG=false` 时会排除测试用户 [6,7,1000] 发布的求购单
- **逐行调 `PurchaseOrderStockLogic::getNum($id)`** 算 `available_amount`（N+1 查询）
- **WANTED 状态下会在分页之后过滤掉 `available_amount <= 0` 的行** ——
  也就是那一页可能少于 `page_size` 条，而 `count()` 不做这个过滤，
  **count 与 lists 天然对不上**。这是原实现的行为，迁移时保持，但要写进文档。

### 4.2 分页参数的两个语义坑

`BaseDataLists::initPage` 里 `page_type` 和 `page_no` 用的是**不同的默认值规则**：

```php
$this->pageType = (int)$this->request->get('page_type', 1);   // 参数缺失才取默认
$this->pageNo   = $this->request->get('page_no', 1) ?: 1;     // falsy 也取默认
```

- `?page_type=0` → 显式 0 → **不分页**（一次取 1,000,000 条）
- `?page_no=0` → falsy → **落回 1**

我第一版把 `page_type=0` 也当成缺失处理，落回了默认的 1（分页）。
后果是**调用方请求全量数据只会拿到 25 条** —— 这是单测抓出来的，不是看代码看出来的。

另外注意 `page_type` **默认是 1（分页）**，这与 likeadmin 通用版本相反。

---

### Stage 3 第六～八批：费率、缓存、以及一个攒了 13 轮的 bug

这一节的每一条都是**跑真机才发现的**，不是读代码推出来的。

#### 🔴 费率计算里有一条读代码推不出来的规则（涉及真金白银）

`FeeAmountLogic::calculateFeeRate` 的第一行：

```php
if (bccomp($tradePrice, '0', 2) === 0) { return '0.00'; }
//                ↑ scale = 2
```

`bccomp` 把两边**截断到 2 位小数**再比。所以 `'0.008'` 被当成 `'0.00'`，
与 `'0'` 相等，**直接返回 `'0.00'`，根本不走后面的向上取整**。

实际规则是：**价格 < 0.01 一律返回 0.00**。边界实测：
`0.0001 / 0.001 / 0.002 / 0.005 / 0.008` 全 `0.00`，`0.01` 才开始 `0.01`。

我按代码推导 `(rate=0.0600, price=0.001)` 时得出 `0.01`，真机给的是 `0.00`。
**参数命名是"价格"，谁会想到它按分比较？**

另外两条同样只有实测才确认：

| 规则 | 实测 |
|---|---|
| `ceil((float)$multiplied)` 先转 **double** 再取整 | Go 侧刻意用 `ParseFloat` **复刻**精度损失，而不是"顺手修好" |
| 最低 0.01 兜底使零费率也收费 | `calculateFeeRate('0','100') === '0.01'` —— rate=0 **仍然收 0.01** |

`showTotalAmount` 还有两条：

- 参数不合法时返回的是 **`'0'`**（没有小数位），合法路径才是 2 位小数字符串。
  这决定 JSON 里是 `"0"` 还是 `"0.00"`。而且 `empty("0")` 与 `empty("0.0")` 不一样：
  前者靠 `empty` 拦下，后者靠 `<= 0`。
- 总价是**向上取整到分**（×100 → ceil → ÷100），不是四舍五入；手续费在取整后的总价上算。

线上配置：`outFee='6.66'`（**6.66%**，不是 6%）。

> 单测期望值全部取自 PHP CLI 真机输出（这两个函数是纯 bcmath，CLI 跑得动）。
> `showTotalAmount` 整链依赖 `Cache::get`（redis 驱动，CLI 跑不了），
> 所以在 PHP 里**复刻函数体、只把 `getFeeRate` 换成固定入参**来采集对照。

#### 🔴 列表接口的分页/筛选参数被静默忽略（攒了 13 轮）

`purchase` / `sales` / `purchase-out` / `purchase-on` 四个接口的
`page_no`/`page_size`/`sort`/`keyword`/`price_*` **全部无效**，一律退回默认第一页 25 条。

**根因**：service 里用 `ctx.(khttp.Context)` 取原始请求。Kratos 的中间件会用
`context.WithValue` 包装 ctx，包装后的动态类型是 `*context.valueCtx`，
**不再是那个实现了 `http.Context` 的 wrapper**，断言必然失败。

**是我当时的注释把它掩盖了**：我在失败分支上写"断言失败也不影响可用性：
退化成没传参数，即默认分页第一页"。这句话技术上对，但**把功能错误说成了可接受的降级**。

**怎么暴露的**：接缓存时算缓存键，发现哈希是 `d41d8cd98f00b204e9800998ecf8427e`
—— 这是**空字符串的 md5**。一个哈希长得像正常哈希，差点让我错过它。

**为什么之前两次测试都没抓到**：第 4 轮测 `purchase` 列表时交易时段关闭，
走的是**硬编码 `page_size=10`** 的分支，根本没经过参数解析；第 15/16 轮测
`purchase/out`、`on` 时没传分页参数。**两次都"通过"了，但都没走到那条路径。**

**修复**：`httpx.WithQuery` / `httpx.QueryFrom` + `server.RequestQueryMiddleware`，
在**最外层中间件**（ctx 还是 wrapper 时）取出 query 存进 context。
⚠️ **顺序必须在 `recovery`/`logging` 之前**，错了会静默退化成空 query。

**同时补了 5 项"参数确实生效"的断言**（冒烟 64 → 69）。原来 64 项全绿却没发现这个 bug，
问题不在覆盖广度，在**断言的类型**——清一色是"键存在""类型对""值等于常量"，
**没有一条是"我传进去的参数改变了我看到的结果"**。

> 教训：测试通过 ≠ 被测行为正确。**先问"这条断言在错误实现上会不会失败"，再问"它现在过不过"。**
> 同一个病在这个项目里出现过三次：期望值抄自自己的错误实现（第 10 轮）、
> 用例没编进去却以为在跑（第 12 轮）、断言类型覆盖不到行为（本轮）。

#### 🔴 软删除过滤：ThinkPHP 隐式加，GORM 不会

`MarketPurchase`、`PurchaseOrders` 都用了 `SoftDelete`，PHP 侧所有查询会隐式加
`delete_time IS NULL`，**GORM 不会**。我在 `checkExchange` 的计数里漏了它，
后果是软删除的兑换单也被算进"当前是否存在兑换单"，用户被**错误拦下**。

实测 `x_market_purchase` 共 750 行，其中 **65 行软删除**。发现线索是
`PurchaseStateLists` 源码第 90 行的注释里贴了一段真实 SQL，末尾带着
`AND MarketPurchase.delete_time IS NULL`——**这条线索一直明写在原项目注释里**。

⚠️ `delete_time` 的类型**并不统一**：`x_market_purchase` 是 `datetime`，
`x_user` 是 `int unsigned`。判空写法一样，类型不同，模型里别混用。
反之 `WarehouseDetail` 继承的是 `think\Model`（非 BaseModel），**表里没有 delete_time**，
不需要过滤——**只能逐个核实，不能套用**。

#### PHP 宽容语义：三个必须显式复刻的地方

| 写法 | 陷阱 |
|---|---|
| `$stockNum == null`（`$stockNum=(int)redis->get()`） | **`0 == null` 为 true** —— "键不存在"和"库存恰好为 0"都走 `bcsub` 兜底。所以 `available_amount` 是**字符串 `"0"`** 而不是数字 `0` |
| `empty("0")` vs `empty("0.0")` | 前者 true、后者 false（后者靠 `<= 0` 拦下）。两条路径不同 |
| `$list['images'] ?: []` | images 为空串时是 `[]`（数组），不是 `null`；而 `grab_time` 为 NULL 时是 `null` |

`StockNum` 因此返回 `(value, exists)` 而不是裸 `int`——**直接返回 int 会丢掉这个语义**。

#### 两个同名不同物的 `getArchive`

`MarketListPurchaseLogic::getArchive` 和 `SaleLogic::getArchive` **都用缓存键 `'archive:{id}'`**，
但完全是两回事：

| | `MarketListPurchaseLogic` | `SaleLogic` |
|---|---|---|
| 缓存机制 | `RedisLockService`（裸 Redis） | `cache()`（TP 缓存） |
| 物理键 | `archive:{id}` | **`la:archive:{id}`** |
| 字段 | **7 个**（含 `collection_id`/`platform_id`） | **5 个**（没有这两个） |
| TTL | 600s | 3600s |
| Go 侧 | **可共用** | **必须隔离** |

物理键不同所以不互相覆盖，但**字段集不同**——谁"顺手统一"这两个函数，
就会让其中一个接口静默拿到错的字段。

#### 两套缓存机制，处理方式相反

| 机制 | 形态 | 迁移处理 |
|---|---|---|
| `cache()`（ThinkPHP） | `la:` 前缀 + **TP 自己的序列化格式** | **必须隔离**（`xtravel:go:` 前缀） |
| `RedisLockService` | **裸 phpredis、无前缀、值就是 `json_encode`** | **可与 PHP 共用同一把键** |

代码里用 `biz.Cache`（带前缀）和 `biz.RawKeyCache`（不带前缀）两个类型区分——
方法集相同，但语义相反，**签名上一眼可见**。用独立类型也解决了 wire
"multiple bindings for biz.Cache" 的报错。

⚠️ 当前 `salesOn` 的 SWR 缓存落到 `xtravel:go:` 而非裸键，与原实现不自洽（已记录，待统一）。

#### GORM 的 `Table()` 不套用表前缀

`Table("app_archive AS app_archive")` **不会**加 `NamingStrategy.TablePrefix`，
用字符串拼表名时必须自己补，否则会去查不存在的 `app_archive`。
这个项目里已经被"表前缀"咬了三次。前缀默认值是 `la_`，而本项目实际是 `x_`——
**配错就是查错表，最难一眼看出的错误**。

## 二、快速开始

```bash
# 1. 配置：直接从 xTravel 的 .env 生成（含真实凭据，产物已 gitignore）
./scripts/gen-config.ps1          # Windows
make gen-config

# 2. 生成代码（改了 .proto 之后）
./scripts/gen.ps1                 # Windows
make api                          # Linux/macOS

# 3. 生成依赖注入（改了构造函数签名之后）
wire ./cmd/xtravel

# 4. 跑
go run ./cmd/xtravel -conf configs/config.local.yaml

# 5. 端到端冒烟（22 项，覆盖信封/鉴权/字段类型）
./scripts/smoke.ps1
```

**不要手工拼 `configs/config.local.yaml` 的 DSN。** 用 `gen-config.ps1`，
它把两个已经踩过的坑固化在代码里（见第九节）：PowerShell 的
`"$db?charset"` 变量名陷阱、以及 `Set-Content -Encoding utf8` 写 BOM 的问题。
脚本写完还会回读校验一遍 DSN 结构。

`configs/config.local.yaml` 含明文数据库密码，**已在 .gitignore 中排除**，不要提交。

---

## 三、目录结构

```
api/xtravel/v1/                 API 定义（proto 是一等公民，不是文档）
  common.proto / user.proto / market.proto
  envelope.go                   ★ 让部分 reply 以「顶层数组」作为 data
cmd/xtravel/
  main.go                        装配 + 优雅退出
  wire.go                        wire 声明（wireinject 标签，不参与普通编译）
  wire_gen.go                    wire 生成的实现（要提交）
configs/
  config.yaml                    可提交的模板
  config.local.yaml              真实凭据（gitignore）
internal/
  conf/conf.proto                配置结构（proto 定义 -> conf.pb.go）
  biz/                           业务逻辑（不依赖任何框架/DB/HTTP）
    phpval.go                    ★ PHP 隐式类型语义的精确复刻
    config.go                    ConfigService::get 的逐行对照实现
    trade.go                     TradeConfigService 的逐行对照实现
    token.go                     ★ LoginMiddleware + UserTokenService + UserTokenCache
    user.go                      UserLogic::info
    market.go                    PurchaseController::payWay/checkExchange
    errors.go                    BizError（可展示给用户的业务错误）
    *_test.go                    单测（PHP 语义回归 + token 生命周期）
  data/                          数据访问（GORM / Redis）
    token.go                     session 仓储 + token 缓存（键空间与 TP 隔离）
    user.go                      x_user / x_user_real / x_user_accounts
    market.go                    x_market_purchase
    lock.go                      ★ Redis 锁（键与 PHP **共用**，见下）
  service/                       协议转换层（proto <-> biz）
  server/                        HTTP 服务装配
    middleware.go                ★ 鉴权中间件（替代 LoginMiddleware）
  pkg/httpx/                     ★ 响应信封（协议兼容核心）
  pkg/pbconv/                    Go 值 <-> protobuf.Value
third_party/                     protoc 依赖的 google/api 等 proto
scripts/
  gen.ps1                        生成 proto 代码
  gen-config.ps1                 ★ 从 .env 生成 config.local.yaml（避开两个已知坑）
  smoke.ps1                      ★ 端到端冒烟测试（22 项断言）
```

**分层铁律**：`service` 只做协议转换，业务判断全在 `biz`；
`biz` 定义接口，`data` 实现（依赖倒置）。原项目里 controller 直接 `Service::get()`
的写法在这里对应到 `biz.ConfigUsecase`。

---

## 四、协议兼容（改这块之前务必读完）

原项目响应信封由 `app/common/service/JsonService.php` 定义，有**两个反直觉点**，
用 Kratos 默认行为会直接打挂前端：

| | 原 ThinkPHP | Kratos 默认 |
|---|---|---|
| 成功码 | **`code = 1`** | `code = 0` |
| HTTP 状态 | **恒为 200**，业务码只在 body | 跟随错误码 |
| 信封 | `{code, show, msg, data}` | `{code, message, data}` |

所以 `internal/pkg/httpx/response.go` 替换了 Kratos 的
`ResponseEncoder` / `ErrorEncoder`。

**另一个隐蔽的坑**：`protojson` 默认**省略零值**（0/""/[]/null），
而 PHP 的 `json_encode` 会全部输出。不打开 `EmitUnpopulated`，
前端会发现字段"凭空少掉"，这是最难查的一类问题。已在 encoder 里打开。

> `httpx.CompatHTTPStatus` 控制是否强制 HTTP 200。
> 现在为 `true`（兼容优先）。等前端切到新契约后改成 `false`，
> 网关/监控才能看到真实的 HTTP 错误 —— 现在的代价是所有失败都"看起来是 200"。

### 4.1 线上契约的四条规则（每个新接口都要过一遍）

这四条是**逐接口**的，不能靠全局约定。每一条我都踩过一次，而且**全都是静默失败**
（不报错，只是前端拿不到数据或算错），所以写在这里当检查清单。

**① JSON 键名：camelCase 还是 snake_case，取决于原 PHP 怎么写**

| 原 PHP 写法 | 键名 | 例子 |
|---|---|---|
| 手写数组 `['outFee' => ...]` | **camelCase** | `outFee` `tradeSwitch` `isOpen` `limitTime` |
| `$model->toArray()` 透传 DB 行 | **snake_case** | `real_name` `create_time` `user_money` `has_password` |
| 手写数组 `['pay_way' => ...]` | **snake_case** | `pay_way` `pay_name` |

protojson 默认输出 camelCase，所以 **snake_case 的字段必须在 `.proto` 里逐个写
`json_name`**：

```proto
string real_name = 5 [json_name = "real_name"];
```

漏写的后果：前端拿到 `realName`，取 `data.real_name` 得到 undefined。

**② 64 位整数必须是 JSON 数字，不是字符串**

proto3 的 JSON 映射**规定** int64/uint64 序列化成**字符串**（为避开 JS 2^53 精度问题）。
但 PHP 的 `json_encode` 对 PHP int 输出**数字**。实测原 PHP：

```json
{"id":61089517,"sn":178056289917980,"create_time":1783678044}
```

已在 `httpx.unquoteInt64` 里用 protoreflect **按字段类型**还原成数字
（不做字符串替换 —— 那会误伤 `user_money` 这种"长得像数字的字符串"）。

**③ 可空列必须是 `null`，不是 `""`**

proto3 的普通 `string` 字段永远是 `""`，区分不出 NULL。用
`google.protobuf.StringValue`：字段不设置 → `null`，设置了 → 字符串。

实例：`x_user.opt_pwd` 可空，实测 **1365 个用户里 1277 个是 NULL**（93%）。
用普通 string 会让这 93% 的用户拿到 `""` 而不是 `null`。

**④ `data` 是数组还是对象**

原项目不少接口 `return $this->data([...])`，`data` 就是**顶层数组**；
成功但无内容时是**空数组 `[]`** 而不是空对象 `{}`。

Kratos 的 reply 是 message，默认会被包成 `{"data":{"items":[...]}}`。
解决办法见 `api/xtravel/v1/envelope.go` —— 给 reply 加一个 `Envelope()` 方法
（Go 的接口是结构化的，不需要 import httpx 就能满足 `httpx.Enveloper`）：

```go
func (r *GetPayWayReply) Envelope() (int, int, string, any) {
    return 1, 0, "", r.GetItems()   // 把 items 摊出去，data 就成了数组
}
```

**验证方式**：`scripts/smoke.ps1` 对这四条都有断言（47 项）。
更好的办法是像 Stage 3 那样，**用真实 PHP 跑一遍同样的查询再逐字段比对** ——
`PDO::ATTR_STRINGIFY_FETCHES=false` + `ATTR_EMULATE_PREPARES=false`，
这样才知道 PHP 到底输出数字还是字符串。

---

### 4.4 错误文案逐字核对（已全量验证过，改动前请重跑）

面向用户的 `fail()` 文案**必须与 PHP 逐字相同** —— 前端直接展示 `msg`，
差一个字用户看到的提示就不同。而这类问题**没有任何测试会抓到**，
除非专门为每条文案写断言。

核对方法（一次性、可重跑）：把 `xTravel/app` 下所有 `.php` 读成一个大字符串，
对 Go 侧每条文案做 `Contains`：

```powershell
$all = (Get-ChildItem "$p\app" -Recurse -Filter *.php | ForEach-Object {
  [IO.File]::ReadAllText($_.FullName, [Text.Encoding]::UTF8) }) -join "`n"
$all.Contains('未找到档案信息')
```

**核对结果（13/13 逐字存在，无一条是自行编造的）：**

| 文案 | PHP 中出现次数 |
|---|---|
| `ID不能为空` | 46 |
| `记录不存在` | 13 |
| `登录过期` | 9 |
| `档案ID不能为空` | 5 |
| `请勿重复操作` | 4 |
| `未找到档案信息` | 2 |
| `未找到艺术品信息` | 2 |
| `登录超时，请重新登录` | 2 |
| `当前不在兑换时间内，暂无法导入数字艺术品` | 1 |
| `当前存在兑换单，暂无法导入数字艺术品` | 1 |
| `请求认证信息有误，请重新登录` | 1 |
| `白名单用户-暂无该操作权限` | 1 |
| `当前未配置交易时段` | 1 |

**说明**：`当前未配置交易时段` 在两边都是**拼接出来的**完整文案，不是纯前缀：

```php
// PHP  TradeConfigService.php
$data['tips'] = '当前未配置交易时段('. (getTodayType()=='workday' ? '工作日':'节假日') .')';
```
```go
// Go   biz/trade.go
b.WriteString("当前未配置交易时段(")
b.WriteString(st.TodayTypeStr)
b.WriteString(")")
```

（写这一节时我最初写的是"Go 只用了前缀、是有意截断"——**那是错的**，
查了双方源码才发现两边拼法一致。凡是关于"哪里不一样"的断言，都要落到源码上。）

新增接口时，把新的 `fail()` 文案加进这个列表并重跑核对。
**不要凭感觉写中文提示。**

### 4.5 Redis 键名逐字核对（已全量验证过）

**共享的键**（锁、`RedisLockService` 缓存）拼错一个字符就会**静默失效**：
防重复点击不再生效、或两边各写各的缓存 —— 不会报错，只会"看起来正常"。

核对方法同 4.4（读全树做 `Contains`）。检查了两遍：
先抓所有含冒号的字符串字面量，再单独抓 `Sprintf` 拼出来的那些
（第一遍会漏掉它们，因为可变部分用 `%d`/`%s` 占位）。

**结果：全部一致。**

| 键 | 机制 | PHP 出现 |
|---|---|---|
| `lock:` | 裸 Redis，共用 | 18 |
| `click:` | 裸 Redis，共用（防重复点击） | 4 |
| `app:` | 裸 Redis，共用（费率缓存） | 5 |
| `archive:` | 裸 Redis，共用（档案缓存） | 2 |
| `user:stock:lookAll:` | 裸 Redis，共用（库存汇总） | 1 |
| `purchase:indexList:` | 裸 Redis，共用 | 1 |
| `purchase:salesOn:` | 裸 Redis，共用（SWR） | 1 |
| `purchase:salesOut:` | 裸 Redis，共用 | 1 |
| `x_PurchaseOrder:Stock:lock:` | 裸 Redis，共用（库存数） | 1 |
| `trade:market:periods` | TP 缓存，隔离 | 1 |
| `trade:market:todayType` | TP 缓存，隔离 | 1 |
| `whitelist:user_perm:` | TP 缓存，隔离 | 2 |
| `config:` | TP 缓存，隔离 | 3 |
| `token_user_` | TP 缓存，隔离 | 1 |
| `user:exchange:check:` | 裸 Redis，共用 | 1 |
| `xtravel:go:` | **Go 自有前缀**，PHP 里没有 | 0（预期） |

两条**预期为 0** 的，不是漏项：

- `x_PurchaseOrder:Stock:lock:{purchaseId}`、`warehouse:unlock:goods:` ——
  PHP 用变量/常量拼接（`"...lock:$purchaseId"`、`'warehouse:unlock:goods'` 再拼 `':'`），
  所以带完整后辍的字面量搜不到。已分别对照源码确认。
- `project:` —— 那是 `config('project.{type}.{name}')` 的**配置数组**查找，不是 Redis 键。

> 顺带提醒：`column:*` 那一大片是 GORM 的 struct tag（`gorm:"column:id"`），
> 不是 Redis 键。用"含冒号"去抓会误捕，看结果时别被吓到。


## 五、鉴权机制（Stage 2）

对应原项目的三个文件，逐行对照实现：

| 原 PHP | Go |
|---|---|
| `app/api/http/middleware/LoginMiddleware.php` | `internal/server/middleware.go` |
| `app/api/service/UserTokenService.php` | `internal/biz/token.go`（Usecase） |
| `app/common/cache/UserTokenCache.php` | `internal/biz/token.go` + `internal/data/token.go` |

流程（与 PHP 完全一致）：

```
读 header `token`
  ├─ 无 token 且需要登录  → code=-403 show=0 "请求认证信息有误，请重新登录"
  ├─ 查缓存 token_user_{token}，未命中 → 查 x_user_session（要求 expire_time > now）+ x_user
  ├─ 无效/过期 且需要登录 → code=-403 show=0 "登录超时，请重新登录"
  ├─ 距过期 < 1 小时 → 自动续期；续期失败 → code=-403 show=1 "登录过期"
  └─ 通过 → 用户信息注入 context（service 层用 biz.UserIDFromContext 取）
```

**两个必须保住的细节**：失败的业务码是 **`-403`**（不是 0），且前两个 `show=0` ——
前端靠 `show` 决定"不弹提示、直接跳登录页"。改成 0 或 show=1 都会改变前端行为。
HTTP 状态码仍是 200，由 `httpx.ErrorEncoder` 统一处理。

**Stage 2 只做「校验 PHP 签发的 token」**：token 的源头是 `x_user_session` 表，
两边都能读，所以 PHP 登录、Go 校验是完全可行的，两套系统可以并行跑。
登录签发（`SetToken`）也一并实现了，但还没接 HTTP 接口。

> 未做线上验证的一项：**自动续期**只在 token 距过期 1 小时内才触发，
> 现有测试库的活跃 token 都在 7 天后过期，所以没在真库上跑过这条路径。
> 逻辑由 `TestOvertimeToken_ExtendsExpiry` 单测覆盖。

---

## 六、为什么有 `phpval.go`

`ConfigService::get` 的分支完全建立在 PHP 的隐式类型转换上：

```php
if ($value) { return $value; }              // "0" 是假，"false" 是真，[] 是假
if ($value === 0 || $value === '0') {...}   // 严格比较
```

用 Go 的直觉去写（比如把 `""` 当有效值）会让配置读取行为和线上不一致，
而且极难排查。所以 `internal/biz/phpval.go` 提供了
`PhpTruthy` / `PhpIsZeroLike` / `PhpJSONDecode` / `DecodeConfigValue`，
把 PHP 语义显式化。**迁移其它 Service 时优先复用这几个函数。**

---

## 七、已知偏差（有意为之，不是漏做）

1. **`IndexLogic::tradeConfig` 的 20 秒 JSON 缓存没实现。**
   `ConfigService::get` 自己已有 60 秒缓存，再叠一层会让配置改动等两轮才生效，
   而且多一份缓存多一处不一致。QPS 打不住时再用同一套 `biz.Cache` 补。

2. **Redis 键前默认加 `xtravel:go:`**（`REDIS_CACHE_PREFIX` 可覆盖）。
   原项目用 ThinkPHP 的 `cache()`，同一个逻辑键（如 `trade:market:periods`）
   存的是 TP 自己的序列化格式。迁移期两套系统并行，共用键会互相读出乱码。
   PHP 下线后再考虑合并。

3. **`config/project.php` 的兜底未迁移。**
   `ConfigService::get` 最后会回退到 `config('project.{type}.{name}')`。
   这里改成读 `configs/project.yaml`（`PROJECT_CONFIG` 环境变量指定）。
   文件不存在时该分支返回 `null` —— 线上如果 `x_config` 表里都有记录，
   根本走不到这一步。**做 Stage 2 时需确认这一点。**

4. **gRPC 端口占位。** `conf.proto` 里定义了 grpc 段但没起 gRPC 服务。
   原项目只有 HTTP，先保持最小面。

5. **免登录白名单从「控制器属性」改成了「operation 全路径」。**
   原项目在每个控制器上声明 `$notNeedLogin = ['index', ...]`，
   Kratos 没有"控制器对象"这个概念，所以在 `internal/server/middleware.go`
   维护一份 operation 白名单（`/xtravel.v1.CommonService/GetConfig` 这种）。
   改 proto 的 service/method 名时要同步改这里，否则接口会突然要求登录。
   配置里的 `auth.public_operations` 是**追加**而不是替换，防止配置漏写把接口全锁死。

6. **Redis 键空间与 ThinkPHP 隔离**（`xtravel:go:` 前缀）。
   token 缓存尤其重要：原实现走 `BaseCache::set` → `store()->tag()->set()`，
   存进 Redis 的是 ThinkPHP 的 tag 结构 + 序列化格式，共用键必然读出乱码。
   （这就是之前 `only array cache can be push` 事故的同一套机制。）

7. **登录签发（`setToken`）已实现但未接线。** 没有对应的 HTTP 接口 ——
   登录属于 user 域后续的工作。Stage 2 的目标是「能校验 PHP 签发的 token」，
   这个已经达成，两套系统可以并行跑。

8. **Redis 前缀：缓存隔离，锁共用 —— 这两个决策是相反的，不是笔误。**

   | 用途 | 前缀 | 为什么 |
   |---|---|---|
   | 缓存（`config:*`、`token_user_*`、`user:info:*`） | `xtravel:go:` **隔离** | 原实现走 `BaseCache::set` → `store()->tag()->set()`，存的是 ThinkPHP 的 tag 结构 + 序列化数据，Go 读不了 |
   | 锁（`click:*`） | 无前缀，**与 PHP 共用** | 锁值只是个随机 token，`SET key val NX PX ms`，两边格式完全一致；而共用才能保证用户不会在新旧两套系统上各提交一次 |

   加错方向的代价：缓存共用会读出乱码；锁隔离会让"防重复点击"在迁移期失效。

9. **`market` 域迁了 11/44 条路由，且全部是只读。** 剩下的里有下单、支付、
   兑换、划转等**涉及资金**的写操作，必须单独评估（幂等、并发、事务边界、
   与 PHP 并存时的双写问题），不能顺手一起做。

10. **兑换单列表里读库存 Redis 失败时，不再让整个接口失败。**
    原实现在 `PurchaseStateLists::lists()` 里逐行调
    `PurchaseOrderStockLogic::getNum($id)`，而那个调用在 try/catch 之内 ——
    Redis 一抖，controller 的 `catch (\Exception)` 接住 → 整个列表返回 fail()。
    这里改成**记日志后走 `bcsub($amount, $receive_amount)` 兜底**。

    **这是行为差异**：极端情况下原系统报错，Go 返回数据（库存字段退化）。
    理由：`available_amount` 只是展示用的一个字段，不该让整个列表挂掉。
    如果要求与原实现逐字一致，改回返回错误即可。

11. **`stale-while-revalidate` 的缓存键与 PHP 不共享（但键空间已对齐）。**
    `salesOn` 的 SWR 键是裸键（`purchase:salesOn:{md5}`，与 `RedisLockService` 同空间），
    但**哈希算法与 PHP 不同**：PHP 是 `md5(json_encode($_GET))`（保留数组插入顺序），
    这里是按键名排序后拼接再取 md5。所以同一个请求两边各存一份缓存。

    取舍理由：为了共用而精确复刻 PHP 的 `json_encode` 字节序（转义、Unicode 处理）
    代价和风险都更高，收益只是省一份缓存。**不会互相破坏，只是不共享。**
    注意 PHP 自己在 `salesOn` 里做了 `ksort($params)`、`salesOut` 里没做 ——
    连它自己都不一致。

---

## 八、迁移过程中发现的原系统问题

### 🔴 `GET /v1/user/info` 把支付密码哈希返回给了前端

`UserLogic::info` 的字段列表里带了 `opt_pwd`，然后只 `hidden(['password'])`：

```php
$user = User::where(['id' => $userId])
    ->field('id,sn,sex,password,nickname,real_name,avatar,mobile,create_time,user_money,opt_pwd')
    ->findOrEmpty();
$user->hidden(['password']);   // ← 只藏了 password，opt_pwd 漏了
```

也就是说**登录用户的支付密码哈希会随接口返回**。同一份响应里已经有
`has_opt_pwd` 布尔字段，前端正常只需要它。

Go 版**暂时保留**该字段（迁移期不改接口契约，避免前端解析出错），
但在 proto 和代码里都标注了。**建议单独提一个改动把它去掉**，
去掉前先确认前端没有用到 `data.opt_pwd`。

### 🔴 软删除用户的 token 仍然能通过鉴权（Go 版已修正）

`UserTokenCache::setUserInfo` 里：

```php
$user = User::where('id','=',$userSession->user_id)->find();  // 软删除的用户查不到，$user 为 null
$userInfo = ['user_id' => $user->id, ...];                    // PHP 8 警告，$user->id 得到 null
$this->set($key, $userInfo, $ttl);
return $this->getUserInfo($token);                            // 非空数组为真 → 鉴权通过
```

用户即使已注销（`delete_time` 非空），只要 `x_user_session` 还有未过期记录，
请求就会**带着 user_id=null 通过鉴权**，然后业务层拿到 userId=0 去做查询。

Go 版改成返回 nil（视为 token 无效）→ 直接 403「登录超时，请重新登录」。
这是**有意的行为收紧**，已在 `internal/biz/token.go` 注明，
并有 `TestSetUserInfo_SoftDeletedUserIsRejected` 锁住。

### 🔴 `create_token()` 在 Go 里照搬会生成重复 token（已修正）

原实现只用 `uniqid()`（微秒）做熵。Go 里直接照搬时，
`time.Now().Nanosecond()/1000` 在 Windows 上因时钟分辨率粗，
同一滴答内连续调用拿到完全相同的熵 —— **实测 200 次循环就出现重复**。

token 列有 UNIQUE 约束，撞了轻则登录失败，重则在换发路径上覆盖到别人的会话。
Go 版额外加了 `crypto/rand` + 进程内自增序号，
`TestCreateToken_Is32HexAndUnique` 会跑 200 次检查唯一性。

### 🟡 白名单 `checkPermission` 的注释与代码互相矛盾（潜伏缺陷）

```php
// 如果限制项不存在于系统中，默认放行
if (!isset($permissions[$itemCode])) {
    return true;        // ← 注释说"放行"，但调用方把 true 当"拦截"
}
```

`WhitelistMiddleware` 里 `if (checkPermissionWithContext(...)) { return fail(...); }`，
也就是**返回 true = 拦截**。所以「限制项不在启用列表里」实际会让**所有人被拦**。

**当前不构成线上故障**：路由用到的 3 个 code（`no_collection_trade` /
`no_tea_trade` / `no_tao_trade`）在 `x_whitelist_item` 里都存在且 `status=1`，
走不到这个分支。

**Go 版按注释的意图实现（未知限制项 → 放行）**，理由：
1. 注释明确写了意图，代码大概率是笔误；
2. 照搬字面行为会在「某天有人把限制项停用」时让整个接口对所有人 403，
   这个失败模式远比"少拦一次"严重。

这是一处**有意的行为分歧**，有单测锁住（`TestWhitelist_UnknownItemIsAllowed`）。
如果业务确认要保留原字面行为，改 `biz/whitelist.go` 里那一行即可。

### 🔴 `workdayPeriods` 与 `tradeTips` 不一致 —— 下午盘可能永远不开

直查 `x_config` 表（`xmarket_test`）：

```
workdayPeriods   [{"start":"09:30","end":"11:30"}]          ← 只有一场
holidayPeriods   [{"start":"13:30","end":"20:00"}]
tradeTips        兑换时段为上午09:30-11:30，下午13:30-15:30；周末及节假日暂停兑换！
```

`isTradingOpen()` 只在 `workdayPeriods` 的时段内返回 1。工作日配置里**没有下午场**，
所以**下午 13:30-15:30 永远开不了市**，但文案明确告诉用户有这个时段。

Go 版与数据库一致（忠实迁移），所以这是**原系统的数据/配置问题，不是迁移引入的**。
需要业务确认：是配置漏了，还是产品逻辑变了。
**生产库 `xmarket` 要单独核实**，测试库不代表线上。

### 🟡 `isTradingOpen()` 忽略全局开关

`isTradingOpen()` 的注释块明确写着：

```
// 平台交易开关 (产品要求:1=关闭交易,0=正常交易)
// 1. 全局总开关（0=开启, 1=关闭）
```

**但代码从未读取 `ConfigService::get('trade','tradeSwitch')`。**
只要在时段内就返回 1，全局开关形同虚设。

Go 版**刻意保留**该行为（见 `internal/biz/trade.go` 注释），保证迁移期新旧判断一致。
要修请单独提改动，不要顺手改 —— 否则新旧系统判断会不一致。

### 🟡 字段名与内容不符

`ConfigController::index` 返回的 `tradeSwitch` 字段，装的是
`TradeConfigService::checkTradeTime()` 的**完整对象**，不是布尔开关。
Go 版保持原样以兼容前端，但在 proto 里写了注释说明。

---

## 九、待迁接口的侦察记录（省下一次重新读源码）

这一节记录已经读过、但**还没到能动手实现**的接口。目的是让下一次接手的人
不必从零开始读源码。所有事实都来自实际读代码 / 跑真机，不是推测。

### `GET /v1/market/purchase/price_range`

调用链：`PurchaseController::priceRange` → `GoodsLogic::priceRange($params)`
→ `AppArchiveLogic::findAppArchives(intval($archive_id))`
→ `PurchaseCreateLogic::getPriceScope($archive)`

返回 `['max_start' => $startPrice, 'max_end' => $endPrice]`；
查不到档案时 `fail('艺术品不存在')`，`getPriceScope` 抛异常时 `fail($e->getMessage())`。

⚠️ **`AppArchiveLogic` 有两个同名文件**：`app/api/logic/open/AppArchiveLogic.php`
（这个才是调用链上的）和 `app/adminapi/logic/app/AppArchiveLogic.php`。
按文件名搜会搜错——`findAppArchives` 只在 `api/logic/open/` 那份里。

`getPriceScope` 的复杂度（约 100 行，是这块的真正工作量）：

- 读 8 个档案字段：`is_local` / `low_price` / `local_price` / `high_price` /
  `price_limit` / `low_percent` / `high_percent` / `id`
- `is_local === 0`（第三方）时要知道当前最高成交价：取
  `ReceiveData::order("create_time","desc")->value("price")`，
  缓存在**裸 Redis** 键 `purchase:ymprice`、TTL 3600 —— Go 可与 PHP 共用
- `is_local === 1`（本地）时基准价改用 `local_price`
- 百分比要 `bcdiv((string)$x, '100', 2)` 转成小数；`base_price` 常量 `'0.01'`
- **两处会抛异常**：`$high_price !== null && $cal_high_price == 0` → `'后台数据异常!'`；
  以及"无最低限价"分支里 `bccomp($purchaseStart, $cal_high_price, 2) > 0` → `'后台数据异常!!'`
  （注意一个是单感叹号、一个是双感叹号，文案不同）
- 然后是一棵按 `price_limit == 0` / `low_price`/`high_price` 是否为 null 展开的分支树，
  大量 `bccomp(..., 2)` 两两取大/取小

**建议**：这块要单独立项，先照着源码把分支树画出来，再逐条与 PHP 对照取样。
不要用"边写边猜"的方式做——它有两处异常分支，很容易把"抛异常"写成"返回默认值"。

### `GET /v1/market/purchase/detail`

调用链：`PurchaseController::detail` → `MarketListPurchaseLogic::getSaleDetail($id, $userId)`。

🔴 **已知缺陷：跨用户缓存泄漏。** 缓存键是 `"purchase:getSaleDetail:$id"`，
**不含 `$userId`**，但响应的计算过程用了 `$userId`（`able_exchange` 之类依赖用户）。
也就是说先访问的用户会把结果缓存下来，后续**其他用户**拿到的是别人的视图。

迁移时必须正面处理：要么把 `$userId` 加进键（修掉），要么保持原样并明确记录。
**不要默默修掉也不要默默保留** —— 两种都需要先确认线上是否已有人依赖当前行为。

（同一个文件里的 `getArchive` 也有类似形态，但它的响应不依赖用户，
所以只有本函数是真问题。区别见第 "两个同名不同物的 getArchive" 一节。）

## 十、环境相关

- Go **1.26.3**，GOPROXY 已指向 `goproxy.cn`
- protoc **35.1**，插件：`protoc-gen-go` / `protoc-gen-go-http` / `protoc-gen-go-grpc`
- **Windows 上 protoc 不能用 `--plugin=<绝对路径>`**，会报
  `filename, directory name, or volume label syntax is incorrect`。
  必须让插件从 PATH 解析 —— `scripts/gen.ps1` 已处理
- **PowerShell 陷阱**：`"$db?charset=..."` 里 `?` 是合法变量名字符，
  `$db?charset` 会被整体当成未定义变量 → 空串。拼接 DSN 必须写 `${db}?charset=`
  （这个坑实际导致过一次连库失败）
- **PowerShell 5.1 的 `Set-Content -Encoding utf8` 会写 BOM**，
  YAML 解析器遇到 BOM 会挂。用 `[System.IO.File]::WriteAllText(..., UTF8Encoding($false))`
- 真实连接信息在 `xTravel/.env` 的 `[DATABASE]` / `[redis]` 段：
  表前缀是 **`x_`**，不是 `config/database.php` 里的默认值 `la_`
