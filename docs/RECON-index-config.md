# 待迁规格：`GET /v1/index/config`（`IndexLogic::getConfigData`）

> 静态分析 + **真机数据**结果，**未实现**。目的是让接手的人不必重读源码、重查数据库。

## 路由与鉴权

- 路径：**`GET /v1/index/config`**（group `/v1/index`，`index.php`）
- 控制器：`IndexController::config` → `IndexLogic::getConfigData()`
- **免登录**（`IndexController::$notNeedLogin` 含 `'config'`）

## 原实现（`app/api/logic/IndexLogic.php:112`）

```php
public static function getConfigData(): array
{
    $tabbar = DecorateTabbar::getTabbarLists();
    $style  = ConfigService::get('tabbar', 'style', config('project.decorate.tabbar_style'));

    $loginConfig = [
        'login_way'       => ConfigService::get('login','login_way',       config('project.login.login_way')),
        'coerce_mobile'   => ConfigService::get('login','coerce_mobile',   config('project.login.coerce_mobile')),
        'login_agreement' => ConfigService::get('login','login_agreement', config('project.login.login_agreement')),
        'third_auth'      => ConfigService::get('login','third_auth',      config('project.login.third_auth')),
        'wechat_auth'     => ConfigService::get('login','wechat_auth',     config('project.login.wechat_auth')),
        'qq_auth'         => ConfigService::get('login','qq_auth',         config('project.login.qq_auth')),
    ];

    $website = [
        'h5_favicon' => FileService::getFileUrl(ConfigService::get('website','h5_favicon')),
        'shop_name'  => ConfigService::get('website','shop_name'),
        'shop_logo'  => FileService::getFileUrl(ConfigService::get('website','shop_logo')),
    ];

    $webPage = [
        'status'      => ConfigService::get('web_page','status', 1),
        'page_status' => ConfigService::get('web_page','page_status', 0),
        'page_url'    => ConfigService::get('web_page','page_url', ''),
        'url'         => request()->domain() . '/mobile',
    ];

    $copyright = ConfigService::get('copyright', 'config', []);

    return [
        'domain'    => FileService::getFileUrl(),      // ⚠️ 无参数
        'style'     => $style,
        'tabbar'    => $tabbar,
        'login'     => $loginConfig,
        'website'   => $website,
        'webPage'   => $webPage,
        'version'   => config('project.version'),
        'copyright' => $copyright,
    ];
}
```

`DecorateTabbar::getTabbarLists()`：

```php
$tabbar = self::select()->toArray();          // 无 where，**全表**（含 is_show=0）
if (empty($tabbar)) { return $tabbar; }
foreach ($tabbar as &$item) {
    if (!empty($item['selected']))   { $item['selected']   = FileService::getFileUrl($item['selected']); }
    if (!empty($item['unselected'])) { $item['unselected'] = FileService::getFileUrl($item['unselected']); }
}
return $tabbar;
```

## 🔴 五个必须注意的点

| # | 点 | 说明 |
|---|---|---|
| 1 | **`FileService::getFileUrl()` 无参数** | `getFileUrl('')` 在 local 驱动下等于 `format(request()->domain(), '')` = **`"<domain>/"`（带尾斜杠）**。所以 `domain` 字段形如 `http://127.0.0.1:18000/` |
| 2 | **`style` 是 json_decode 后的对象** | `ConfigService::get` 对字符串值会 `json_decode`，实测 `tabbar.style` 存的是 `{"default_color":"#999999","selected_color":"#c455ff"}` → 输出是**对象**，不是字符串 |
| 3 | **`tabbar` 不是数组而是"对象数组"** | `link` 列本身是 JSON 字符串，但模型没声明 json 转换 → 输出**原始字符串**。而 `selected`/`unselected` 经 `getFileUrl` 变成完整 URL |
| 4 | **`tabbar` 是全表** | 没有 `is_show` 过滤。实测 3 行都是 `is_show=1`，但**代码确实没过滤** |
| 5 | **`webPage.url` 用请求域名** | `request()->domain() . '/mobile'` —— 与 `avatar` 一样，**必须从请求推导**，不能用配置 |

## 真机数据（本项目库，2026-10-09 实测）

### `x_config` 里存在的相关行

| type | name | value |
|---|---|---|
| `tabbar` | `style` | `{"default_color":"#999999","selected_color":"#c455ff"}` |
| `website` | `h5_favicon` | `resource/image/adminapi/default/web_favicon.ico` |
| `website` | `shop_name` | `数藏市场` |
| `website` | `shop_logo` | `resource/image/adminapi/default/shop_logo.png` |

### **不存在的段**（所以走 `config/project.php` 或方法内默认值）

| 配置 | 结果 |
|---|---|
| `login` 段 | **无行** → 6 个值全部来自 `config/project.php` |
| `web_page` 段 | **无行** → `status=1`、`page_status=0`、`page_url=''`（方法内默认值） |
| `copyright` 段 | **无行** → `[]`（空数组） |

### `config/project.php` 的兜底值

```php
'version' => '1.9.4',
'login' => [
    'login_way'       => ['1', '2'],   // ⚠️ 字符串数组
    'coerce_mobile'   => 1,
    'third_auth'      => 1,
    'wechat_auth'     => 1,
    'qq_auth'         => 0,
    'login_agreement' => 1,
],
```

### `x_decorate_tabbar`（3 行，全表）

| id | name | selected | unselected | link | is_show | create_time | update_time |
|---|---|---|---|---|---|---|---|
| 13 | 首页 | `resource/.../tabbar_home_sel.png` | `resource/.../tabbar_home.png` | `{"path":"\/pages\/index\/index","name":"\u5546\u57ce\u9996\u9875","type":"shop"}` | 1 | 1779516710 | 1779516710 |
| 14 | 资讯 | `resource/.../tabbar_text_sel.png` | `resource/.../tabbar_text.png` | `{"path":"\/pages\/news\/news",...}` | 1 | 1779516710 | 1779516710 |
| 15 | 我的 | `resource/.../tabbar_me_sel.png` | `resource/.../tabbar_me.png` | `{"path":"\/pages\/user\/user",...}` | 1 | 1779516710 | 1779516710 |

列：`id, name, selected, unselected, link, is_show, create_time, update_time`（**无 delete_time** → 无软删除）

## 预期的对外形态（按当前库数据）

```json
{
  "code": 1, "show": 0, "msg": "",
  "data": {
    "domain": "http://127.0.0.1:18000/",
    "style": {"default_color":"#999999","selected_color":"#c455ff"},
    "tabbar": [{"id":13,"name":"首页","selected":"http://.../tabbar_home_sel.png",
                "unselected":"http://.../tabbar_home.png",
                "link":"{\"path\":\"\\/pages\\/index\\/index\",...}",
                "is_show":1,"create_time":1779516710,"update_time":1779516710}, ...],
    "login": {"login_way":["1","2"],"coerce_mobile":1,"login_agreement":1,
              "third_auth":1,"wechat_auth":1,"qq_auth":0},
    "website": {"h5_favicon":"http://.../web_favicon.ico","shop_name":"数藏市场",
                "shop_logo":"http://.../shop_logo.png"},
    "webPage": {"status":1,"page_status":0,"page_url":"","url":"http://127.0.0.1:18000/mobile"},
    "version": "1.9.4",
    "copyright": []
  }
}
```

## 实现前还需确认

| # | 项 |
|---|---|
| 1 | `project.decorate.tabbar_style` 的实际值（`style` 的兜底；本次没读到） |
| 2 | `config('project.version')` = `'1.9.4'` —— 已确认 |
| 3 | `login_way` 是**字符串数组** `['1','2']` → 响应里就是字符串 `["1","2"]`，**不要转成数字** |
| 4 | `copyright` 为空数组时 `EmitUnpopulated` 下的输出形态（`[]` vs `{}`）—— 原实现是 PHP 空数组，`json_encode` 是 `[]` |

## 为什么这轮没做

写这个接口需要 4 段配置读取 + 1 张表查询 + 6 种可空/类型差异，
而本次会话的上下文预算已接近耗尽。**在预算耗尽时写复杂代码的风险是产出错误实现
（本会话已发生过一次 proto 被改乱的事故）**，所以选择先把已有的真机数据固定下来。
