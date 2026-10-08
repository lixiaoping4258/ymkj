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

## ✅ 关键未知项已解决：密码加密算法（19:20 查证）

**不是 PHP 的 `password_hash`，是项目自定义的 md5 双重拼接**，Go 侧可轻松复刻。

`app/common.php:16`：

```php
function create_password(string $plaintext, string $salt): string
{
    return md5($salt . md5($plaintext . $salt));
}
```

校验处 `app/api/validate/LoginAccountValidate.php:162-163`：

```php
$passwordSalt = Config::get('project.unique_identification');
if ($userInfo['password'] !== create_password($password, $passwordSalt)) { /* 密码错误 */ }
```

**Go 实现（3 行）**：

```go
func CreatePassword(plaintext, salt string) string {
    inner := md5.Sum([]byte(plaintext + salt))
    outer := md5.Sum([]byte(salt + hex.EncodeToString(inner[:])))
    return hex.EncodeToString(outer[:])
}
```

> ⚠️ **注意内外两层的编码**：内层 `md5()` 在 PHP 里默认返回 **32 位十六进制字符串**
> （`raw_output=false`），所以外层拼的是 hex 串，再 md5 一次。写成拼原始字节会得到不同结果。

**盐已经有了**：`Config::get('project.unique_identification')` 对应 `.env` 的
`UNIQUE_IDENTIFICATION`，我已经在 `biz.AuthConfig.UniqueIdent` 里持有（长度 7）。
**不需要新增配置或依赖。**

### 这条对第 1 批的意义

| 原担心 | 实际 |
|---|---|
| 可能是 PHP 专有哈希，Go 无法复现，只能改成调 PHP | **是普通 md5，可原生复刻** |
| 需要额外的哈希库或跨语言调用 | 只需标准库 `crypto/md5` |

**结论：第 1 批（`account` 密码登录 + `logout`）技术上没有障碍，可以独立完成。**

### 另一个发现：登录会自动注册

`LoginLogic::login` 里，当 `UserAccounts` 查不到时，会**走注册流程**
（`$registerParams = ['channel'=>..., 'account'=>..., 'password'=>Id::uuid()]`）。
也就是说 **`account`（登录）不是纯只读接口，它会写库**——迁移时要按写操作对待
（幂等、并发、事务）。

## ⚠️ 迁移前仍须确认的问题（原 4 条，第 1 条已解决）

2. **短信/验证码通道**：`SmsController::sendCode` 用的是哪家？有无 SDK？
3. **微信配置从哪来**：`WeChatConfigService` 读的是 `x_config` 表还是配置文件？
4. **登录失败锁定**：原实现有无重试次数/锁定逻辑？（`login` 152 行，很可能有）

## 结论

登录是 C 端的入口，**没有它整个 C 端跑不起来**，所以优先级应该高于 `market` 剩余部分。
但它**不是一个可以顺手带走的接口** —— 607 行逻辑 + 一套微信集成。
建议先做**第 1 批（`account` + `logout`）**，把"密码校验算法"这个关键未知项解决掉，再决定后续。