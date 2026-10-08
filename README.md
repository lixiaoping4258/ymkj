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
| Stage 2 | 鉴权中间件（`UserTokenService` → JWT）+ `user` 域 | 待做 |
| Stage 3 | `market` 域（45 条路由，核心业务） | 待做 |
| Stage 4 | `wallet` / `payment` / `ticket` / `whitelist` | 待做 |
| Stage 5 | `adminapi`（61 控制器，最大一块） | 待做 |
| Stage 6 | `open` / `third` / 队列消费者 | 待做 |

### 验证记录（真实数据库 `xmarket_test`）

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

---

## 二、快速开始

```bash
# 1. 配置：复制并填入真实连接信息
cp configs/config.yaml configs/config.local.yaml     # 后者已 gitignore

# 2. 生成代码（改了 .proto 之后）
./scripts/gen.ps1            # Windows
make api                     # Linux/macOS

# 3. 生成依赖注入（改了构造函数签名之后）
wire ./cmd/xtravel

# 4. 跑
go run ./cmd/xtravel -conf configs/config.local.yaml
curl http://127.0.0.1:18000/v1/common/config
```

`configs/config.local.yaml` 含明文数据库密码，**已在 .gitignore 中排除**，不要提交。

---

## 三、目录结构

```
api/xtravel/v1/common.proto     API 定义（proto 是一等公民，不是文档）
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
  data/                          数据访问（GORM / Redis）
  service/                       协议转换层（proto <-> biz）
  server/                        HTTP 服务装配
  pkg/httpx/                     ★ 响应信封（协议兼容核心）
  pkg/pbconv/                    Go 值 <-> protobuf.Value
third_party/                     protoc 依赖的 google/api 等 proto
scripts/gen.ps1                  Windows 代码生成
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

---

## 五、为什么有 `phpval.go`

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

## 六、已知偏差（有意为之，不是漏做）

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

---

## 七、迁移过程中发现的原系统问题

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

## 八、环境相关

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
