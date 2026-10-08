# 待迁侦察：`v1/account` 登录域（C 端入口）

> 静态分析结果，**未实现**。目的：摸清成本，避免开了一个做不完的工。

## 规模

| 层 | 文件 | 行数 |
|---|---|---|
| 控制器 | `app/api/controller/v1/account/LoginController.php` | **263**（13 个方法） |
| 逻辑 | `app/api/logic/LoginLogic.php` | **607**（14 个方法） |
| 外部服务 | `WeChatConfigService` / `WeChatMnpService` / `WeChatOaService` / `WeChatRequestService` | 未测 |
| 校验器 | `LoginAccountValidate` / `RegisterValidate` / `WechatLoginValidate` / `WebScanLoginValidate` | 未测 |

## LoginLogic 方法粒度（移植成本的主要构成）

| 方法 | 行数 | 是否依赖微信 |
|---|---|---|
| `login` | **152** | 否 |
| `register` | 77 | 否 |
| `scanLogin` | 42 | **是**（网页扫码） |
| `createAuth` | 34 | **是** |
| `oaLogin` | 32 | **是**（公众号） |
| `getScanCode` | 31 | **是** |
| `mnpLogin` | 31 | **是**（小程序） |
| `silentLogin` | 30 | **是** |
| `mnpAuthLogin` | 27 | **是** |
| `oaAuthLogin` | 27 | **是** |
| `updateLoginInfo` | 24 | 否 |
| `logout` | 21 | 否 |
| `codeUrl` | 16 | **是** |
| `updateUser` | 9 | **是** |

**依赖微信的约 300 行（9 个方法），不依赖的约 280 行（5 个方法）。**

## 依赖清单

**可以直接复用（已迁）**：
- `app\api\service\UserTokenService` → 已有 `biz.UserTokenUsecase`（token 签发/续期/过期）

**需要新移植**：
- `app\api\service\WechatUserService` + 4 个 `common\service\wechat\*`（微信小程序/公众号/网页扫码）
- `app\common\cache\WebScanLoginCache`（网页扫码登录状态）
- `xlu\Id`（ID 生成）、`LoginEnum` / `UserTerminalEnum` / `YesNoEnum`
- `GuzzleHttp` → Go 侧用 `net/http` 或已有客户端

## 建议的拆分（**不要一次性做完**）

| 批次 | 内容 | 依赖 | 可行性 |
|---|---|---|---|
| **1** | `account`（密码登录）+ `logout` | 无微信；`UserTokenService` 已就绪 | **可做**，约 175 行逻辑 |
| **2** | `register` + `sendCode` + `captcha` | 短信/图形验证码 | 需确认短信通道 |
| **3** | `thirdAuth` / `thirdLogin` / `thirdCheck` | 控制器里**无外部依赖** | 待确认是哪种第三方 |
| **4** | 微信全系列（小程序/公众号/网页扫码） | 4 个 WeChat Service + Guzzle | **成本最高，建议单独立项** |

## ⚠️ 迁移前必须先确认的问题

1. **密码加密算法**：`LoginLogic::login` 里怎么校验密码？如果是 PHP 专有的哈希（如 `password_hash` 的 bcrypt 或项目自定义），Go 侧要么复用同一算法，要么改成调 PHP——**这会决定第 1 批是否真能独立完成**。
2. **短信/验证码通道**：`SmsController::sendCode` 用的是哪家？有无 SDK？
3. **微信配置从哪来**：`WeChatConfigService` 读的是 `x_config` 表还是配置文件？
4. **登录失败锁定**：原实现有无重试次数/锁定逻辑？（`login` 152 行，很可能有）

## 结论

登录是 C 端的入口，**没有它整个 C 端跑不起来**，所以优先级应该高于 `market` 剩余部分。
但它**不是一个可以顺手带走的接口** —— 607 行逻辑 + 一套微信集成。
建议先做**第 1 批（`account` + `logout`）**，把"密码校验算法"这个关键未知项解决掉，再决定后续。