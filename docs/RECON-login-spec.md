# 待迁规格：`LoginLogic::login`（152 行）

> 逐行读源码得出的分支规格，**未实现**。这是实现前的最后一步侦察。

## ⚠️ 最重要的一点：密码校验**不在这个方法里**

`login()` **假定密码已经验过了**。真正的密码比对发生在**验证器层**：

`app/api/validate/LoginAccountValidate.php`
```php
$passwordSalt = Config::get('project.unique_identification');
if ($userInfo['password'] !== create_password($password, $passwordSalt)) { /* 密码错误 */ }
```

**所以 Go 侧必须复刻这个分层**：handler 先跑校验（含密码比对），再调 `login`。
把密码校验塞进 `login` 里会改变错误信息的产生位置和顺序。

`create_password` 已在 `biz/password.go` 实现并用 PHP 真机验证（6/6 通过）。

## 三个 scene 的入口分支

| scene | 查 `UserAccounts` 的条件 | mobile | type |
|---|---|---|---|
| `ACCOUNT_PASSWORD`（账号密码） | `{app_id: 1, type: 1, account}` | `''` | 1 |
| `MOBILE_CAPTCHA`（手机验证码） | `{app_id: 1, type: 2, account}` | `account` | 2 |
| `THIRD_LOGIN`（第三方） | `{app_id: params.app_id, type: 3, account}` | `params.mobile` | 3 |
| 其它 | — | — | — |

其它值 → `fail('不支持的登录方式')` + `return false`

## 主流程（按分支顺序）

### 分支 A：`UserAccounts` 查不到 → **自动注册**

```php
$registerParams = ['channel' => $params['terminal'], 'account' => $params['account'],
                   'password' => Id::uuid()];
if ($params['scene'] == THIRD_LOGIN) {
    $registerParams['extData'] = array_merge(['app_id' => $params['app_id']], $params['extData']);
}
$result = self::register($registerParams, $type, $mobile);
if ($result === false) { Log::error('注册失败'); return false; }   // ← 注意：不 setError
$userId = $result['user_id'];
$isCreate = true;
```

- 注册用的密码是 **`Id::uuid()`**（随机），不是用户输入——所以账号密码场景下第一次登录会建一个随机密码的账号
- 注册失败时**只记日志、不设错误信息**（`setError` 没被调用），上层拿到的 msg 会是空的

### 分支 B：`UserAccounts` 有值

```php
if ($account->state != 'ENABLE') { self::setError('账号当前不可用'); return false; }
$userId = $account->user_id;
$isCreate = false;
```

### 汇合后的校验

```php
$user = User::where(['id' => $userId])->findOrEmpty();
if ($user->isEmpty())  { self::setError('不存在的用户信息'); return false; }
if ($user->is_disable == 1) { self::setError('账号冻结中，无法登录'); return false; }
```

> ⚠️ `User::where(...)` 在 PHP 侧带 ThinkPHP 的隐式软删除过滤（见 README 第七节）。

### 副作用（写库）

```php
$user->login_time = time();
$user->login_ip   = request()->ip();
$user->save();
```

**`login_ip` 取自 `request()->ip()`** —— Go 侧要从 HTTP 请求取，注意 PHP 会考虑
代理头（`X-Forwarded-For` 等）。**这条要单独核对**，否则记进库的 IP 会不一致。

### 签发 token

```php
$userInfo = UserTokenService::setToken($user->id, $params['terminal']);
```

→ **已迁**：`biz.UserTokenUsecase.SetUserInfo`（含缓存写入）。

### 头像

```php
$avatar = $user->avatar ?: Config::get('project.default_image.user_avatar');
$avatar = FileService::getFileUrl($avatar);
```

- 空头像回退到 `project.default_image.user_avatar`
- `FileService::getFileUrl` 要做**存储前缀拼接**（本项目用 OSS/本地存储），**未迁，需单独确认规则**

### 条件副作用：第三方通知（仅 `app_id == 2`）

```php
if ($params['app_id'] == 2) {
    Event::trigger('ThirdNotify', ['action' => 'account.register', 'app_id' => ..., 'user_id' => ...,
        'body' => ['err'=>'OK', 'data'=>['mobile'=>..., 'uid'=>..., 'custom_uid'=>$params['account']],
                   'msg' => $isCreate ? '用户注册成功' : '用户登录成功'],
        'request_id' => 0]);
}
```

**只针对 `app_id == 2`（元梦空间 2）**，是向外部系统发通知。`Event::trigger`
的监听器在别处，**需要单独找出来看做了什么 HTTP 调用**。

### 返回值

```php
return ['nickname' => $userInfo['nickname'], 'sn' => $userInfo['sn'],
        'mobile' => $userInfo['mobile'], 'avatar' => $avatar, 'token' => $userInfo['token']];
```

注意 `mobile` 取自 **`$userInfo`（token 缓存里的值）**，不是 `$user`。

### 异常兜底

```php
catch (\Exception $e) { Log::error('登录异常'.$e->getMessage()); self::setError($e->getMessage()); return false; }
```

**异常信息会直接返回给前端**（`setError($e->getMessage())` → `fail(msg)`）。
这是原实现的既有行为，按"不改动原逻辑"要求保留。

## 实现前还需确认的 3 件事

| # | 项 | 为什么关键 |
|---|---|---|
| 1 | **`register()` 的实现**（`LoginLogic::register` 77 行） | 分支 A 依赖它；它内部还会建 `UserAccounts`、`User` 等记录，是**多表写入**，需要事务边界设计 |
| 2 | **`FileService::getFileUrl`** | 头像 URL 拼接规则，影响响应内容 |
| 3 | **`ThirdNotify` 监听器** | `app_id == 2` 时会发外部通知，需知道发到哪、失败怎么办 |

## 已有可复用件

| 依赖 | 状态 |
|---|---|
| `create_password` | ✅ `biz/password.go`（PHP 真机验证 6/6） |
| `UserTokenService::setToken` | ✅ `biz.UserTokenUsecase.SetUserInfo` |
| 盐 `UNIQUE_IDENTIFICATION` | ✅ `biz.AuthConfig.UniqueIdent` |
| `UserTerminalEnum`（terminal） | ⬜ 未迁（值需从 PHP 枚举抄） |
| `LoginEnum`（scene 常量） | ⬜ 未迁 |
