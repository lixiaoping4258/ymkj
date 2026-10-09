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
- 注册失败时 `login()` 只记日志（`Log::error`），但 **`register()` 内部已调用 `setError`**，
  所以上层仍能拿到错误信息（详见下方"更正"一节）

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

## 附：枚举值与 `getFileUrl` 规则（19:40 补查）

### `LoginEnum`（scene）

```php
const ACCOUNT_PASSWORD = 1;
const MOBILE_CAPTCHA   = 2;
const THIRD_LOGIN      = 3;
```

> 注意：`login()` 的 `switch` 用的是这些**整数常量**，但 `UserAccounts.type`
> 里存的也是 1/2/3，两者含义不同但数值恰好对应，**实现时别混用同一个常量**。

### `UserTerminalEnum`（terminal）

```php
const WECHAT_MMP = 1;   // 微信小程序
const WECHAT_OA  = 2;   // 微信公众号
const H5         = 3;   // 手机H5登录
const PC         = 4;   // 电脑PC
const IOS        = 5;   // 苹果app
const ANDROID    = 6;   // 安卓app
// OTHER = 0 被注释掉了
```

> 实测库里的活跃会话 `terminal = 3`（H5），与 `x_user_session.terminal` 一致。

### `FileService::getFileUrl($uri, $type)`

```php
if (strstr($uri, 'http://'))  return $uri;      // 已是完整 URL 直接返回
if (strstr($uri, 'https://')) return $uri;
$default = Cache::get('STORAGE_DEFAULT');       // 存储驱动：local / oss / ...
if (!$default) { $default = ConfigService::get('storage','default','local');
                 Cache::set('STORAGE_DEFAULT', $default); }
if ($default === 'local') { ... }
```

要点：
- 已带 `http://` / `https://` 的**原样返回**（不做任何处理）
- 存储驱动从 `ConfigService::get('storage','default','local')` 读，并缓存到 **TP 缓存**
  的 `STORAGE_DEFAULT` 键
- `$type == 'public_path'` 时返回 `public_path() . $uri`
- **`local` 之外的分支（oss 等）未读完** —— 实现前要看完整，否则头像 URL 拼接会不一致
## 附：`LoginLogic::register`（77 行，行 55-120）—— **多表写入，带事务**

这是分支 A 的实现基础，也是**唯一涉及事务的地方**。

```php
public static function register(array $params, int $accountType = 1, string $mobile = ''): bool|array
{
    Db::startTrans();                       // ← 事务
    try {
        $userSn       = User::createUserSn();
        $passwordSalt = Config::get('project.unique_identification');
        $password     = create_password($params['password'], $passwordSalt);
        $avatar       = ConfigService::get('default_image', 'user_avatar');
        $data = ['sn'=>$userSn, 'avatar'=>$avatar, 'nickname'=>'用户'.$userSn,
                 'password'=>$password, 'channel'=>$params['channel'], 'mobile'=>$mobile];
        if ($accountType == 3 && isset($params['extData'])) {   // 第三方
            $data['avatar']   = $params['extData']['avatar']   ?? $avatar;
            $data['nickname'] = $params['extData']['nickname'] ?? ('用户'.$userSn);
            $app_id = $params['extData']['app_id'];
        } else { $app_id = 1; }
        $user = User::create($data);
        if (!$user) { Db::rollback(); self::setError('用户注册失败'); return false; }
        $account = ['id'=>Id::gen(), 'user_id'=>$user->id, 'account'=>$params['account'],
                    'app_id'=>$app_id, 'type'=>$accountType, 'create_time'=>TimeHelper::now()];
        $result = UserAccounts::create($account);
        if (!$result) { Db::rollback(); self::setError('用户注册失败'); return false; }
        Db::commit();
        return $account;                    // ← 返回的是 UserAccounts，不是 User
    } catch (\Exception $e) {
        Db::rollback(); self::setError($e->getMessage()); return false;
    }
}
```

### 实现要点

| 点 | 说明 |
|---|---|
| **事务** | `User` + `UserAccounts` **两张表**必须同一事务；任一失败 rollback |
| **返回值** | 返回的是 **`UserAccounts` 记录**，调用方取 `$result['user_id']` —— 别返回 User |
| **生成 SN** | `User::createUserSn()`，**未查其算法**，实现前要看 |
| **生成 ID** | `Id::gen()`（`xlu\Id`），用于 `UserAccounts.id` —— 可能是雪花/自定义 ID，**需确认** |
| **默认昵称** | `'用户' . $userSn` |
| **默认头像** | `ConfigService::get('default_image','user_avatar')` —— 注意与 `login()` 里的 `project.default_image.user_avatar` **是不同的配置键**，需分别确认 |
| **第三方覆盖** | 仅 `accountType == 3` 时用 `extData` 覆盖头像/昵称，并取 `extData.app_id`；否则 `app_id = 1` |

### ⚠️ 更正上一节的一处错误推断

上一节我写的是"注册失败时不设错误信息（`setError` 没被调用），上层 msg 会是空的"。

**这是错的。** 读 `register()` 源码后确认：两处失败分支**都调用了 `setError('用户注册失败')`**，
catch 里也调了 `setError($e->getMessage())`。所以注册失败时**错误信息是有的**。

错因：我按"调用方没 setError"下结论，**没去读被调函数**。
教训：**凡涉及跨函数的行为，必须读到被调函数再下结论。**
## 🔴 补查：`User::createUserSn()` 与 `xlu\Id::gen()`（20:00）

### `createUserSn()` 就是 `Id::gen()`

`app/common/model/user/User.php:339` —— 原本的"随机 8 位数字 + 查重"实现**已被整段注释掉**：

```php
public static function createUserSn($prefix = '', $length = 8)
{
    // ...原来的 mt_rand 实现整段被注释...
    return Id::gen();          // ← 实际就是调这个
}
```

所以 **`sn`（用户编号）与 `UserAccounts.id` 用的是同一个生成器**。

### `xlu\Id::gen()` —— 一个基于 **Redis 的分布式 ID 生成器**

`extend/xlu/Id.php`：

```php
public static function gen(int $offset = 0): int
{
    $redisCache = Cache::store('redis');
    $key = 'id:gen';            // ← 注意：经 TP 缓存层，实际键是 la:id:gen
    $prefixKey = 'id:prefix';   //                        la:id:prefix
    try {
        if (!$redisCache->has($key)) { $redisCache->delete($prefixKey); }
        $prefix = $redisCache->get($prefixKey);
        if (!$prefix) { $prefix = time() + $offset; $redisCache->set($prefixKey, $prefix); }
        $suffix = $redisCache->inc($key);
        if ($suffix >= 99995) {     // 5 位用尽 -> 换新的时间前缀
            $redisCache->set($key, 0);
            $redisCache->set($prefixKey, time() + $offset);
        }
        return intval(sprintf('%s%05d', $prefix, $suffix));
    } catch (InvalidArgumentException $e) { return 0; }
}
```

**生成规则**：`<秒级时间戳><5 位零填充的递增序号>`，拼成一个整数。

举例：`1783678044` + `00001` → `178367804400001`（15 位）。
**这与我在库里看到的 ID 形态一致**（如 `178056289901818`、`178609302407853`）。

### 🔴 对迁移的关键影响

**这是一个跨系统共享状态的组件，不是纯函数。**

| 问题 | 说明 |
|---|---|
| **键空间** | 走 `Cache::store('redis')`，所以实际 Redis 键是 **`la:id:gen` / `la:id:prefix`**（TP 缓存前缀）。**我在第 17 轮列 Redis 键时见过这两个键**，当时没意识到它们是 ID 生成器 |
| **Go 必须读同一把键** | 否则两套系统各自生成 ID → **要么冲突、要么（更糟）同一个 ID 被两边各用一次** |
| **前缀是时间戳** | Go 侧要用**秒级** `time.Now().Unix()`，且必须与 PHP 同处一个时钟（同一台机器或 NTP 同步） |
| **重置逻辑** | `suffix >= 99995` 时重置 —— Go 侧必须复刻，否则可能与 PHP 抢同一段序号 |
| **并发** | 靠 Redis `INCR` 的原子性；Go 侧同样用 `INCR` 即可，**不要自己加锁** |

### 迁移建议

**不要重新实现，而是抽成一个共用的 `biz.IDGenerator` 端口**，`data` 层用裸 redis 客户端
对 `la:id:gen` / `la:id:prefix` 做 `INCR` / `GET` / `SET`，**与 PHP 共用同一把键**。

⚠️ 这属于"基础设施必需"而非"业务逻辑改动"，不违反"不改动原项目逻辑"的指令 ——
但**键名必须与 PHP 完全一致**，写错一个字符会导致 ID 重复（这是数据损坏级的问题）。

**实现前必须做**：拿 PHP 生成的 ID 序列与 Go 生成的序列**交叉验证**（交替调用，确认无重复、单调）。
## 补查（续）：`FileService::getFileUrl` 与 `ThirdNotify`（第 2、3 个未知项）

### `FileService::getFileUrl($uri, $type)` —— 完整规则

`app/common/service/FileService.php:42`

```php
if (strstr($uri, 'http://'))  return $uri;       // 已是完整 URL -> 原样返回
if (strstr($uri, 'https://')) return $uri;

$default = Cache::get('STORAGE_DEFAULT');
if (!$default) { $default = ConfigService::get('storage','default','local');
                 Cache::set('STORAGE_DEFAULT', $default); }

if ($default === 'local') {
    if ($type == 'public_path') { return public_path() . $uri; }
    $domain = request()->domain();               // ← 🔴 取自【当前请求】的域名
} else {
    $storage = Cache::get('STORAGE_ENGINE') ?: ConfigService::get('storage', $default);
    $domain  = $storage ? $storage['domain'] : '';
}
return self::format($domain, $uri);
```

`FileService::format($domain, $uri)`（L98）：

```php
// 去掉 domain 末尾的 '/'
if ('/' == substr($domain, -1)) { $domain = substr_replace($domain,'',-1,1); }
// 去掉 uri 开头的 '/'
if ('/' == substr($uri, 0, 1))  { $uri    = substr_replace($uri,'',0,1); }
return trim($domain) . '/' . trim($uri);
```

**🔴 迁移要点**：

| 点 | 说明 |
|---|---|
| **`local` 时域名来自请求** | `request()->domain()` 是**当前请求的域名**（含协议），Go 侧必须从 **`Host` 头 + 协议**推导。**这是请求相关的，不是配置** —— 用配置里的固定域名会导致头像 URL 与实际访问域名不一致 |
| **`STORAGE_DEFAULT` 走 TP 缓存** | 实测该键在 Redis 里是 **`s:5:"local";`（PHP 序列化）**，Go **读不了**。但它的值等价于 `ConfigService::get('storage','default','local')`，**Go 侧直接读配置即可**，不要试图解析那个键 |
| 两张缓存键 | `STORAGE_DEFAULT`（驱动名）、`STORAGE_ENGINE`（驱动详细配置） |
| 已是完整 URL | 带 `http://` / `https://` 的**原样返回**，不做任何处理（所以 `avatar` 若是外部 URL，前缀逻辑完全不参与） |

### `ThirdNotify` → 是**写库**，不是发 HTTP（比我预想的简单）

`app/event.php` 的 `listen` 段：
```php
'ThirdNotify' => [AppNotifyListener::class],
```

`app/common/listener/AppNotifyListener.php`（28 行）：
```php
public function handle($params) {
    $action=..., $app_id=..., $user_id=..., $body=..., $requestId=...;
    $result = OpenAppsNotifyLogic::create($action, $app_id, $user_id, $body, $requestId);
    if (false === $result) { Log::write('三方通知信息入库失败:'...); return false; }
    return true;
}
```

**结论**：`ThirdNotify` 只是**把通知记一条到库里**（`OpenAppsNotifyLogic::create`），
**不发外部 HTTP 请求**。真正的投递应该在别处（队列消费者）。

**对迁移的影响**：
- 登录时 `app_id == 2` 需要**插一条通知记录** → 属于"写操作"，需要与 `login` 的其他写在同一事务语义里考虑
- 失败只记日志、**不影响登录结果**（`handle` 返回 false，但 `LoginLogic` 没检查返回值）
- ⚠️ **`OpenAppsNotifyLogic::create` 的字段与目标表未查** —— 实现前需要看

### 剩余未知项（原 4 项 -> 现 1 项）

| # | 项 | 状态 |
|---|---|---|
| 1 | 密码算法 | ✅ 已解决（`biz/password.go` + 真机 6/6） |
| 2 | `Id::gen` | ✅ 已解决（`data/idgen.go` + 真机交叉验证） |
| 3 | `getFileUrl` | ✅ 本轮解决（含"域名取自请求"这个关键点） |
| 4 | `ThirdNotify` | ✅ 本轮解决（是写库不是 HTTP） |
| **5** | **`MOBILE_CAPTCHA` 的验证码校验** | ⬜ **仍缺** —— 手机验证码登录分支用不了 |

**另有两项实现前需要看（本轮新识别）**：
- `OpenAppsNotifyLogic::create` 的字段与目标表
- `public_path()` 在 Go 侧对应什么（`getFileUrl($uri,'public_path')` 分支）
## 🔴 补查（第 44 轮）：账号密码登录**需要图形验证码**，且有**失败锁定**

之前我只读了 `LoginLogic::login`，**没有读验证器**。补读 `LoginAccountValidate` 后发现两处遗漏，
**都会影响安全性，所以不能在补齐前接线 service**。

### 一、`checkConfig` 要求图形验证码（scene = ACCOUNT_PASSWORD）

`app/api/validate/LoginAccountValidate.php:75`

```php
$config = ConfigService::get('login', 'login_way');
if (!in_array($scene, $config)) { return '不支持的登录方式'; }

switch ($scene) {
    case LoginEnum::MOBILE_CAPTCHA:                     // scene = 2
        if (!isset($data['code'])) { return '请输入手机验证码'; }
        return $this->checkCode($data['code'], [], $data);

    case LoginEnum::ACCOUNT_PASSWORD:                   // scene = 1
        if (!isset($data['password'])) { return '请输入密码'; }
        // 验证图形验证码
        $captchaId = $data['captchaId'] ?? '';
        if (empty($captchaId)) { return '图形验证码ID不正确'; }
        $captcha = $data['captcha'] ?? '';
        if (!CaptchaLogic::verifyCaptcha($captchaId, $captcha)) { return '图形验证码不正确'; }
        return $this->checkPassword($data['password'], [], $data);
}
```

**结论**：`POST /v1/login/account` 在 `scene=1` 时必须带 `captchaId` 与 `captcha`，
否则连密码都不会校验就直接失败。**这是我不知道的依赖**（`CaptchaLogic` + 验证码的生成/存储未迁）。

顺带说明：登录方式本身还受配置控制 —— `ConfigService::get('login','login_way')` 里没有的 scene 直接报"不支持的登录方式"。

### 二、`checkPassword` 有账号安全锁定

`LoginAccountValidate.php:125`

```php
$userAccountSafeCache = new UserAccountSafeCache();
if (!$userAccountSafeCache->isSafe()) {
    return '密码连续' . $userAccountSafeCache->count . '次输入错误，请' . $userAccountSafeCache->minute . '分钟后重试';
}

$condition = ['app_id'=>1, 'type'=>1, 'account'=>$data['account'], 'state'=>'ENABLE'];
$account = UserAccounts::where($condition)->field('user_id')->find();
if (!$account)            { return '用户账户不存在'; }          // ← 注意这条

$userInfo = User::where(['id'=>$account->user_id])->field(['password,is_disable'])->findOrEmpty();
if ($userInfo->isEmpty()) { return '用户不存在'; }
if ($userInfo['is_disable'] === YesNoEnum::YES) { return '用户已禁用'; }
if (empty($userInfo['password'])) { $userAccountSafeCache->record(); return '用户不存在'; }

$passwordSalt = Config::get('project.unique_identification');
if ($userInfo['password'] !== create_password($password, $passwordSalt)) {
    $userAccountSafeCache->record();                        // ← 记一次失败
    return '密码错误';
}
```

**要点**：

| 点 | 说明 |
|---|---|
| **锁定机制存在** | `UserAccountSafeCache`（`isSafe()` / `record()` / `count` / `minute`），**未迁，实现前必须读** |
| 错误信息共 5 条 | `用户账户不存在`、`用户不存在`、`用户已禁用`、`密码错误`、`密码连续N次输入错误，请M分钟后重试` |
| **查询带 `state='ENABLE'`** | 与 `LoginLogic::login` 里的 `state != 'ENABLE'` 检查**重复但位置不同**：这里查不到会报 `用户账户不存在`，而 `login()` 里报 `账号当前不可用`。**同一个账号状态问题，两条路径的提示不同** |
| `$\rightarrowfield(['password,is_disable'])` | ⚠️ 这里写的是**一个字符串** `'password,is_disable'` 而不是数组 —— ThinkPHP 会把整串当成**一个字段名**。实际会怎样需实测（可能查不出 `is_disable`，导致禁用检查失效）。**这是个潜在缺陷，按"不改动原逻辑"应照搬，但必须先实测确认行为** |

### 三、这对已完成的 biz/data 层意味着什么

`internal/biz/login.go` 实现的是 **`LoginLogic::login` 那一层**（密码已校验通过之后的部分），
这一层本身是对的。

**但 handler 还缺三层**：

```
① 参数校验（terminal / scene / account 的 require|in）  ← 未做
② checkConfig：登录方式白名单 + 图形验证码 + 密码比对 + 锁定  ← 未做，且依赖未迁
③ LoginLogic::login  ← ✅ 已做（biz 层）
```

**所以现在接线 service 会做出一个跳过验证码与锁定保护的登录接口 —— 不能接。**

### 四、新增的待办

| # | 项 | 阻塞什么 |
|---|---|---|
| 1 | `CaptchaLogic` + 验证码的生成/存储（Redis 键？） | scene=1 登录 |
| 2 | `UserAccountSafeCache`（锁定计数、时长、键名） | scene=1 登录 |
| 3 | `ConfigService::get('login','login_way')` 的实际值 | 登录方式白名单 |
| 4 | `field(['password,is_disable'])` 的实际行为实测 | 禁用检查是否真的生效 |
| 5 | `SmsController::sendCode` + `checkCode` | scene=2 登录 |
## 补查（第 45 轮）：`UserAccountSafeCache` 与 `CaptchaLogic` —— **键无法共用**

### 一、`UserAccountSafeCache`（`app/common/cache/UserAccountSafeCache.php`，81 行）

```php
class UserAccountSafeCache extends BaseCache
{
    private string $key;       // 缓存次数名称
    public int $minute = 15;   // 锁定 15 分钟
    public int $count  = 15;   // 15 次错误后锁定

    public function __construct() {
        parent::__construct();
        $ip = \request()->ip();
        $this->key = $this->tagName . $ip;      // ← tagName + IP
    }
    public function record(): void {
        if ($this->get($this->key)) { $this->inc($this->key, 1); }     // 已存在 -> 自增
        else { $this->set($this->key, 1, $this->minute * 60); }        // 首次 -> 设 900s TTL
    }
    public function isSafe(): bool { return !($this->get($this->key) >= $this->count); }
    public function relieve(): void { $this->delete($this->key); }
}
```

**要点**：

| 点 | 说明 |
|---|---|
| **按 IP 锁定，不是按账号** | `$this->key = $tagName . request()->ip()` —— 同一 IP 后面所有账号共享计数 |
| **15 次 / 15 分钟** | 与注释一致 |
| **TTL 只在首次设置** | `inc()` 不续期 → 窗口是"第一次失败起的 15 分钟"，不是滑动窗口 |
| `$tagName` | `BaseCache::__construct` 里 `= get_class($this)` = `app\common\cache\UserAccountSafeCache` |

### 二、🔴 `BaseCache::set()` 走 ThinkPHP 的 **tag 机制**

```php
abstract class BaseCache extends Cache          // 继承 think\Cache（不是某个 driver）
{
    public function set($key, $value, $ttl = null): bool {
        return $this->store()->tag($this->tagName)->set($key, $value, $ttl);
    }
}
```

`tag()` 会让 ThinkPHP 用**标签索引 + 键派生**来存，**不是简单的 `前缀 + key`**。
所以 Go 侧**无法用"拼一个键名"的方式共用这个锁定计数**。

### 三、`CaptchaLogic`（`app/api/logic/CaptchaLogic.php`，76 行）

```php
public static function getCaptcha(int $w = 150, int $h = 40): array {
    $cacheKey = sprintf('captcha:%s', $id);
    cache($cacheKey, $phrase, 300);            // TTL 300 秒
    ...
}
public static function verifyCaptcha(string $id, string $captchaValue): bool {
    $cacheKey = sprintf('captcha:%s', $id);
    $captcha = cache($cacheKey);
    cache($cacheKey, null);                    // ← 校验后立即删除（一次性）
    ...
}
```

- 键形如 `captcha:{id}`，走全局 `cache()` 助手（默认 store）
- **TTL 300 秒**
- **校验后立刻删除** —— 一次性使用，重放无效

### 四、缓存驱动确认

```php
// config/cache.php
'default' => env('cache.driver', 'file'),
'stores'  => ['file' => ['prefix'=>'la'], 'redis' => ['prefix'=>'la:']]
```

`.env` 的 `[cache]` 段设了 `driver = redis`，**实测确认**（PHP CLI 打印 `cache.default = redis`、`env CACHE_DRIVER = 'redis'`）。

⚠️ 所以 `config/cache.php` 里那个 `'file'` 默认值**是被 `.env` 覆盖的**，别被它误导。
（Redis 里扫不到 `*captcha*` / `*UserAccountSafe*` 只是因为当前没有存活记录，不是"存在文件里"。）

### 五、🔴 由此得出的结论：**这两个状态 Go 无法与 PHP 共用**

| 状态 | 能否共用 | 原因 |
|---|---|---|
| 锁定计数 | ❌ | 走 TP tag 机制，键名是派生的，不是可拼的 |
| 图形验证码 | ⚠️ 理论上可（键是 `la:captcha:{id}`），但**业务上不该共用** | 验证码**图片是某一侧生成的**；A 侧生成的验证码 B 侧没有对应的明文，除非共用同一个键。而共用又要求两侧对同一 id 的存取完全一致 —— 一旦 Go 也实现 `/v1/captcha` 就会互相覆盖 |

**因此迁移期的行为必然是**：

- **锁定**：PHP 侧 15 次 + Go 侧 15 次 = **同一个 IP 共 30 次机会**。**保护被削弱一半**，这是共存期的固有限制，**必须记录**。
- **验证码**：哪一侧签发 `/v1/captcha`，就必须由同一侧校验登录。**不能一半用 PHP 一半用 Go。**

### 六、这不违反"不改动原项目逻辑"

- **业务规则完全照搬**：15 次 / 15 分钟 / 按 IP / 首次设 TTL / 验证码 TTL 300s / 校验后删除 —— 一条不改
- **变的只是存储位置**（Go 用自己的键），这属于**基础设施必需**，与已有的
  "Redis 键前缀隔离（Go 读不了 ThinkPHP 的序列化格式）"是同一类
- 但**必须把"共存期保护减半"如实记录**，不能悄悄放过

### 七、仍然待查

| # | 项 | 说明 |
|---|---|---|
| 1 | `CaptchaLogic::verifyCaptcha` 的**完整比对逻辑** | 只读了前 4 行，是否大小写敏感 / 是否兼容旧格式未知 |
| 2 | 验证码图片库 `Mickeywaugh\Captcha` | Go 侧需要等价的图形验证码生成 |
| 3 | `UserAccountSafeCache::relieve()` 的**调用点** | 登录成功是否清除计数？没清除的话成功登录后计数仍在 |
| 4 | `ConfigService::get('login','login_way')` 的实际值 | 决定哪些 scene 可用 |
| 5 | `field(['password,is_disable'])` 的实际行为 | 潜在缺陷，需实测 |
## 补查（第 46 轮）：`login_way` 配置与 `relieve()` 调用点

### 一、`UserAccountSafeCache` 的使用点只有一处

全项目搜索（排除 vendor）：

```
app\api\validate\LoginAccountValidate.php    relieve=1  record=2  isSafe=1
app\common\cache\UserAccountSafeCache.php    （定义处）
```

所以：**锁定机制只在登录验证器里生效**，没有别的调用方。

### 二、⚠️ 我上一轮的一个担心是不成立的（更正）

上一轮我发现 `x_config` 表里**没有 `type='login'` 的行**：

```
type        n
recharge    2
sms         2
tabbar      1
trade      10
website     12
```

于是怀疑 `ConfigService::get('login','login_way')` 返回 null，导致 `in_array($scene, null)` 抛 TypeError，
**整个账号密码登录都是坏的**。

**这个推断是错的。** 读完 `ConfigService::get` 的完整实现后发现它有**本地配置文件兜底**：

```php
if ($default_value !== null) { return $default_value; }
// 返回本地配置文件中的值
return config('project.' . $type . '.' . $name);        // ← L93
```

`config/project.php`（L84）：

```php
'login' => [
    // 登录方式：1-账号密码登录；2-手机短信验证码登录
    'login_way'       => ['1', '2'],
    'coerce_mobile'   => 1,
    'third_auth'      => 1,
    'wechat_auth'     => 1,
    'qq_auth'         => 0,
    'login_agreement' => 1,
],
```

**所以 `login_way = ['1','2']`，登录方式白名单是 1 和 2，登录没有坏。**

错因：我看到"DB 里没有配置"就下了"会返回 null"的结论，**没有读完 `ConfigService::get` 的全部返回路径**。
（这与第 41 轮"没读被调函数就推断跨函数行为"是同一类错误 —— **第二次犯**。）

### 三🔴、`in_array` 的**宽松比较**必须复刻

`login_way` 是**字符串数组** `['1','2']`，而 `$scene` 是**整数**（`post()` 提交的 JSON 数字）。

```php
in_array($scene, $config)      // 未传第三个参数 -> $strict = false -> 宽松比较
```

- `in_array(1, ['1','2'])` → **true**（1 == '1' 宽松成立）
- 如果 Go 侧写成严格比较 `scene == 1 || scene == 2`，结果一样；但如果写成"字符串比较"就会全部失败

**实现建议**：直接按整数判断 `scene == 1 || scene == 2`，与宽松比较的结果一致。
**不要**去读那个配置再跟字符串比 —— 那样会因为类型不匹配而全部拒绝。

### 四、顺带确认的两个配置值

| 用途 | 来源 | 值 |
|---|---|---|
| 密码盐 | `project.unique_identification`（`env('project.unique_identification','likeadmin')`） | **实测 `.env` 覆盖为 `aaaa123`**（7 位） |
| 默认头像 | `project.default_image.user_avatar` | `resource/image/adminapi/default/default_avatar.png` |

⚠️ 注意 `login()` 里用的是 `Config::get('project.default_image.user_avatar')`，
而 `register()` 里用的是 `ConfigService::get('default_image','user_avatar')` ——
**两者写法不同但最终都指向 `project.default_image.user_avatar`**（`Config::get` 直接读配置文件，
`ConfigService::get` 走 DB→兜底→同一配置）。**值相同，不需要分别处理。**

### 五、README 第七节第 3 条可以更新

原记录："`config/project.php` 的兜底未迁移"。
**现在至少要迁 `project.login.login_way` 和 `project.default_image.user_avatar` 这两项**，
因为登录链路依赖它们。其余项按需再迁。