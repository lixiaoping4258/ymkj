# 统一路由表（本文件由 scripts/gen-routes.ps1 自动生成，请勿手改）

> **为什么有这份文件**：Kratos 的路由分散在各 `api/xtravel/v1/*.proto` 的
> `option (google.api.http)` 里，没有原 PHP 项目 `app/api/route/*.php` 那样的单一来源。
> 这份表把 proto、免登录白名单两处真相合并，且**从代码生成**，不会脱节。

> 改路由后重新生成：`.\scripts\gen-routes.ps1`

**已实现 24 / 131 条（18.3%）；其中免登录 10 条。**

## 免登录（无需 token）

| 方法 | 路径 | 服务·方法 | proto |
|---|---|---|---|
| GET | `/v1/article/detail` | ArticleService.GetArticleDetail | `article.proto` |
| GET | `/v1/common/captcha` | CaptchaService.GetCaptcha | `captcha.proto` |
| GET | `/v1/common/config` | CommonService.GetConfig | `common.proto` |
| GET | `/v1/common/protocol` | CommonService.GetProtocol | `common.proto` |
| GET | `/v1/index/bannerList` | IndexService.GetBannerList | `index.proto` |
| GET | `/v1/index/decorate` | IndexService.GetDecorate | `index.proto` |
| GET | `/v1/index/index` | IndexService.GetIndex | `index.proto` |
| POST | `/v1/login/account` | AccountService.AccountLogin | `account.proto` |
| GET | `/v1/market/purchase` | MarketService.PurchaseIndex | `market.proto` |
| GET | `/v1/market/sales/categories` | MarketService.GetSaleCategories | `market.proto` |

## 需要 token

| 方法 | 路径 | 服务·方法 | proto |
|---|---|---|---|
| GET | `/v1/article/about` | ArticleService.GetArticleAbout | `article.proto` |
| GET | `/v1/article/licenses` | ArticleService.GetArticleLicenses | `article.proto` |
| GET | `/v1/common/platform/lists` | CommonService.GetPlatformLists | `common.proto` |
| GET | `/v1/common/trade/config` | CommonService.GetTradeConfig | `common.proto` |
| GET | `/v1/market/check/exchange` | MarketService.CheckExchange | `market.proto` |
| GET | `/v1/market/pay_way` | MarketService.GetPayWay | `market.proto` |
| GET | `/v1/market/purchase/info` | MarketService.PurchaseInfo | `market.proto` |
| GET | `/v1/market/purchase/on` | MarketService.PurchaseOn | `market.proto` |
| GET | `/v1/market/purchase/out` | MarketService.PurchaseOut | `market.proto` |
| GET | `/v1/market/purchase/showTotalAmount` | MarketService.ShowTotalAmount | `market.proto` |
| GET | `/v1/market/sales` | MarketService.SaleIndex | `market.proto` |
| GET | `/v1/market/sales/info` | MarketService.SaleInfo | `market.proto` |
| GET | `/v1/market/stock/lookall` | MarketService.StockLookAll | `market.proto` |
| GET | `/v1/user/info` | UserService.GetUserInfo | `user.proto` |

## 接口约定（三条，改动会直接打挂前端）

1. **响应信封**：`{"code":1,"show":0,"msg":"","data":{...}}` —— **`code=1` 才是成功**
   （Kratos 默认 `code=0` 成功，是反的，所以替换了 `ResponseEncoder`）
2. **HTTP 状态码恒为 200**，业务码只在 body 里；失败是 `code:0, show:1`
3. **鉴权**：token 放在名为 **`token`** 的请求头里（不是 `Authorization`）

## 免登录白名单的实现位置

`internal/server/middleware.go` 的 `defaultPublicOps`（map 的 key 是完整的
operation 名 `/<package>.<Service>/<Method>`）。

> ⚠️ 原项目是在控制器上声明 `$notNeedLogin`，Kratos 没有"控制器对象"，
> 只能展开成 operation 全路径。**改名时要同步改这里**，
> 否则免登录判定会静默失效（两个方向都会出事：该公开的变需登录 -> 前端 403；
> 该需登录的变公开 -> 未授权访问）。
