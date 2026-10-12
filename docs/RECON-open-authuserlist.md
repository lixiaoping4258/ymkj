# 待迁侦察：`GET /v1/open/authUserList`（一级平台授权用户列表）

> 静态分析结果，**未实现**。信息已足够，接手可直接照此实现。

## 路由与鉴权

- 路径：**`GET /v1/open/authUserList?app_id=N`**（group `/v1/open`）
- 控制器：`OpenAppController::authUserList`（`$notNeedLogin = []` → **需登录**）

```php
public function authUserList(): Json
{
    (new AuthUserListValidate())->get()->goCheck();
    return $this->dataLists(new AppUserLists());
}
```

> ⚠️ `AuthUserListValidate` 的 **`$rule` 部分没读到**（只看到 `$message` 的结尾 `];`），
> 实现前需补读 `app/api/validate/open/AuthUserListValidate.php` 的规则与文案。

## `AppUserLists`

### `queryWhere()`

```php
$where[] = ['uid', '=', $this->userId];
$where[] = ['app_id', '=', $this->params['app_id']];
// $where[] = ['expire', '>', TimeHelper::now()];   // 🔴 **这行被注释掉了！**
```

🔴 **"授权未过期"的过滤被注释掉了** —— 所以**过期的授权也会返回**。
照搬，不要"顺手"恢复这行。

### `lists()`

```php
$lists = AppAuth::alias("app_auth")->where($this->queryWhere())
    ->field('app_auth.id,app_auth.app_id,app_auth.app_nickname,app_auth.app_avatar,
             app_auth.start,app_auth.expire,u.mobile')
    ->join('user u', 'app_auth.uid = u.id')
    ->order('id', 'desc')                     // ⚠️ 不带表别名的 id
    ->limit($this->limitOffset, $this->limitLength)
    ->select()->toArray();

$app_ids = array_column($lists, 'app_id');
$appLists = (new AppLists())->listsWithQuery([['id', 'in', implode(',', $app_ids)]]);
$appMap = MapHelper::mapWithKey($appLists, 'id');

foreach ($lists as &$list) {
    $list['app_info'] = $appMap[$list['app_id']];        // ⚠️ 见下
    $list['mobile'] = substr_replace($list['mobile'], '****', 3, 4);   // ⚠️ length = 4
}
```

**7 个字段 + 1 个嵌套对象**：
`id, app_id, app_nickname, app_avatar, start, expire, mobile` + `app_info`（对象）

### `count()`

```php
return AppAuth::where($this->queryWhere())->count();
```

🔴 **`count()` 没有任何 JOIN**（`lists()` 有一个 `x_user` 的 INNER JOIN）。
**又一处 lists / count 不一致** —— 与 `AuthMyUserLists` 是同类问题的另一种表现：
若 `uid` 在 `x_user` 里不存在，`lists()` 不返回该行但 `count()` 会计入。

### `detail()`

```php
$appAuth = AppAuth::where('id', $this->params['app_auth_id'])->findOrEmpty()->toArray();
$app = (new AppLists())->detail($appAuth['app_id']);
$appAuth['app_info'] = $app;
return $appAuth;
```

⚠️ 注意 `authUserDetail` 的入参是 **`app_auth_id`**（不是 `app_id`）。
⚠️ 查不到时 `$appAuth['app_id']` 会触发 **undefined index**（PHP 8 下是 Warning，
   然后 `detail(null)` → `App::where('id', null)->findOrEmpty()` → 空数组）。

## `AppLists::listsWithQuery($where)`

```php
return App::where(array_merge($this->queryWhere(), $where))
    ->order('id', 'desc')
    ->limit($this->limitOffset, $this->limitLength)
    ->field('id, name, state, logo, type, fee_rate, flag')
    ->select()->toArray();
```

- `$this->queryWhere()` = `[['state','=',1], ['type','=',2]]`
- 合并进来的是 `[['id','in', implode(',', $app_ids)]]`
- **7 列**：`id, name, state, logo, type, fee_rate, flag`（比 `lists()` 多 `state`）
- ⚠️ **不处理 `logo`**（`lists()` 会，见已实现的 `/v1/open/appList`）
- ⚠️ 用了 `$this->limitOffset/limitLength` —— 但 `AppLists` 是新 new 出来的，
  分页参数来自**当前请求**（`page_no`/`page_size`），所以 `app_info` 最多只取当前页大小条。
  这是个隐藏截断：**同一页里 `app_id` 种类数超过 page_size 时，部分 `app_info` 会缺失。**

## 🔴 `$appMap[$list['app_id']]` 可能不存在

`listsWithQuery` 带了 `state=1 AND type=2` 的条件。如果某个 `app_id` 对应的平台
**已下架（state!=1）或不是三方（type!=2）**，它**不会出现在 `$appMap` 里** ——
PHP 会产生 `Undefined array key` 的 Warning，并把 `app_info` 赋成 **`null`**。

Go 侧要复刻：**查不到时 `app_info` 输出 `null`**（不是 `{}`，也不是省略该键）。

## 掩码对照（**两个接口不同！**）

| 接口 | 调用 | 11 位手机号结果 | 长度 |
|---|---|---|---|
| `authMyUserList` | `substr_replace($m,'****',3,6)` | `183****08` | 11 → **9** |
| `authUserList` | `substr_replace($m,'****',3,4)` | `183****0008` | 11 → **11** |

**两者长度参数不同（6 vs 4），输出形态也不同。** 不要复用同一个函数。

（已用真实 PHP 验证：`length=4` 的结果与原串等长；`length=6` 会短 2 位。）

## 实现要点

| # | 项 |
|---|---|
| 1 | `expire > now()` 的过滤**被注释掉了**，照搬（返回过期授权） |
| 2 | `count()` 无 JOIN，`lists()` 有 —— 照搬不一致 |
| 3 | 掩码 `length=4`（与 `authMyUserList` 的 6 不同） |
| 4 | `app_info` 是**嵌套对象**，7 列，**不处理 logo** |
| 5 | 查不到 `app_info` 时输出 **`null`** |
| 6 | `x_app_auth` 有 `delete_time`；`x_app` 没有；`listsWithQuery` 走 `App` 模型会自动加 |
| 7 | `order('id','desc')` 是不带别名的 `id` —— 与 `x_user` 的 `id` 同时存在，**可能有歧义**（需实测 MySQL 报不报 ambiguous） |
| 8 | `start` / `expire` 都是 **datetime** → 必须 `time.Time` + `Format`（清单第 1 条） |

## 为什么这轮没实现

上下文预算已接近耗尽。**第 7 条的 SQL 歧义需要用真实 PHP 跑一次 `fetchSql` 确认**
（`order('id','desc')` 在 `app_auth JOIN user` 下是否被解析成 `app_auth`.`id`）——
这正是本轮验证 join 前缀所用的方法（见 README 第十一节第 2 条）。
先把已确认的信息固定下来。
