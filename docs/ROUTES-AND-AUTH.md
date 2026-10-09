# C 端 `app/api` 路由 × 免登录 × 迁移状态

> 由静态解析生成：从 16 个路由文件按 **group 分组** 解析 `Route::` 声明
> （**必须跟踪 group 前缀** —— 各文件的 group 各不相同，用文件名当前缀会得到错误路径），
> 再从 33 个控制器抽 `notNeedLogin` 数组交叉得出。

> **共 140 条唯一路由，其中免登录 26 条，已迁 14 条。**

> ⚠️ **这是迁移时最容易标错的地方**，且两个方向都会出事：
> 把该登录的标成公开 = 未授权访问；把该公开的标成需登录 = 前端匿名访问直接 403。
> **不要凭接口名猜** —— 这张表是唯一依据。改 proto 的 service/method 名时要同步改 `internal/server/middleware.go` 的 `defaultPublicOps`。

## 各路由文件的 group 前缀（路径由它决定）

| 文件 | group 前缀 |
|---|---|
| `account.php` | `/v1/login` |
| `article.php` | `/v1/article` |
| `common.php` | `v1/common/` |
| `index.php` | `/v1/index` |
| `market.php` | `v1/market` |
| `open.php` | `v1/open/` |
| `payment.php` | `v1/payment` |
| `search.php` | `/v1/search` |
| `security.php` | `v1/security/` |
| `sms.php` | `/v1/sms` |
| `Tao.php` | `v1/tao` |
| `ticket.php` | `v1/ticket` |
| `upload.php` | `/v1/upload` |
| `user.php` | `/v1/user` |
| `wallet.php` | `v1/wallet` |
| `whitelist.php` | `/v1/whitelist``, ``trade/``, ``wallet/` |

## 全量路由表

| 方法 | 完整路径 | 控制器::方法 | 免登录 | 已迁 |
|---|---|---|---|---|
| POST | `/trade/buy` | TradeController::buy | 否 |  |
| POST | `/trade/sell` | TradeController::sell | 否 |  |
| GET | `/v1/article/about` | ArticleController::about | 否 |  |
| GET | `/v1/article/addCollect` | ArticleController::addCollect | 否 |  |
| GET | `/v1/article/cancelCollect` | ArticleController::cancelCollect | 否 |  |
| GET | `/v1/article/cate` | ArticleController::cate | **是** |  |
| GET | `/v1/article/collect` | ArticleController::collect | 否 |  |
| GET | `/v1/article/detail` | ArticleController::detail | **是** |  |
| GET | `/v1/article/licenses` | ArticleController::license | 否 |  |
| GET | `/v1/article/lists` | ArticleController::lists | **是** |  |
| GET | `/v1/common/captcha` | CaptchaController::index | **是** |  |
| GET | `/v1/common/config` | ConfigController::index | **是** | ✅ |
| GET | `/v1/common/notice` | NoticeController::index | 否 |  |
| GET | `/v1/common/platform/lists` | PlatformController::index | 否 |  |
| GET | `/v1/common/protocol` | IndexController::policy | **是** |  |
| GET | `/v1/common/trade/config` | IndexController::tradeConfig | 否 | ✅ |
| GET | `/v1/index/bannerList` | BannerController::getList | **是** |  |
| GET | `/v1/index/config` | IndexController::config | **是** |  |
| GET | `/v1/index/decorate` | IndexController::decorate | **是** |  |
| GET | `/v1/index/index` | IndexController::index | **是** |  |
| GET | `/v1/index/test` | IndexController::test | **是** |  |
| POST | `/v1/login/account` | LoginController::account | **是** |  |
| GET | `/v1/login/codeUrl` | LoginController::codeUrl | **是** |  |
| GET | `/v1/login/logout` | LoginController::logout | **是** |  |
| POST | `/v1/login/mnpAuthBind` | LoginController::mnpAuthBind | 否 |  |
| POST | `/v1/login/mnpLogin` | LoginController::mnpLogin | **是** |  |
| POST | `/v1/login/oaAuthBind` | LoginController::oaAuthBind | 否 |  |
| POST | `/v1/login/oaLogin` | LoginController::oaLogin | **是** |  |
| POST | `/v1/login/register` | LoginController::register | **是** |  |
| GET | `/v1/login/thirdAuth` | LoginController::thirdAuth | **是** |  |
| GET | `/v1/login/thirdCheck` | LoginController::thirdCheck | **是** |  |
| GET | `/v1/login/thirdLogin` | LoginController::thirdLogin | **是** |  |
| POST | `/v1/login/updateUser` | LoginController::updateUser | 否 |  |
| GET | `/v1/market/check/exchange` | PurchaseController::checkExchange | 否 | ✅ |
| POST | `/v1/market/order/create/purchase` | OrderController::createPurchaseOrder | 否 |  |
| GET | `/v1/market/orders/create/query` | OrderController::queryCreateOrderStatus | 否 |  |
| POST | `/v1/market/orders/create/sale` | OrderController::createSaleOrder | 否 |  |
| POST | `/v1/market/orders/purchase/delete` | PurchaseOrdersController::delete | 否 |  |
| GET | `/v1/market/orders/purchase/detail` | PurchaseOrdersController::orderDetail | 否 |  |
| GET | `/v1/market/orders/purchase/lists` | PurchaseOrdersController::orderLists | 否 |  |
| GET | `/v1/market/orders/purchase/my_lists` | PurchaseOrdersController::myLists | 否 |  |
| POST | `/v1/market/orders/purchase/offline` | PurchaseOrdersController::offline | 否 |  |
| GET | `/v1/market/orders/purchase/saledetail` | PurchaseOrdersController::saleDetail | 否 |  |
| GET | `/v1/market/orders/purchase/salelist` | PurchaseOrdersController::saleList | 否 |  |
| GET | `/v1/market/orders/sale/cancel` | SaleOrdersController::cancelOrder | 否 |  |
| GET | `/v1/market/orders/sale/detail` | SaleOrdersController::detail | 否 |  |
| GET | `/v1/market/orders/sale/goodslist` | SaleOrdersController::goodsList | 否 |  |
| GET | `/v1/market/orders/sale/lists` | SaleOrdersController::orderLists | 否 |  |
| GET | `/v1/market/orders/sale/recorddetail` | SaleOrdersController::recordDetail | 否 |  |
| GET | `/v1/market/orders/sale/saleslist` | SaleOrdersController::salesList | 否 |  |
| GET | `/v1/market/pay_way` | PurchaseController::payWay | 否 | ✅ |
| GET | `/v1/market/price_range` | PurchaseController::priceRange | 否 |  |
| GET | `/v1/market/purchase` | PurchaseController::index | **是** | ✅ |
| POST | `/v1/market/purchase/create` | PurchaseController::create | 否 |  |
| GET | `/v1/market/purchase/detail` | PurchaseController::detail | 否 |  |
| POST | `/v1/market/purchase/grabPriceCheck` | PurchaseController::grabPriceCheck | 否 |  |
| POST | `/v1/market/purchase/grabPriceLook` | PurchaseController::grabPriceLook | 否 |  |
| POST | `/v1/market/purchase/grabPriceSubmit` | PurchaseController::grabPriceSubmit | 否 |  |
| GET | `/v1/market/purchase/info` | PurchaseController::purchaseInfo | 否 | ✅ |
| GET | `/v1/market/purchase/on` | PurchaseController::salesOn | 否 | ✅ |
| GET | `/v1/market/purchase/out` | PurchaseController::salesOut | 否 | ✅ |
| POST | `/v1/market/purchase/purchaseBuy` | PurchaseController::purchaseBuy | 否 |  |
| GET | `/v1/market/purchase/showTotalAmount` | PurchaseOrdersController::showTotalAmount | 否 | ✅ |
| GET | `/v1/market/sales` | SaleController::index | 否 | ✅ |
| POST | `/v1/market/sales/buy` | SaleController::buy | 否 |  |
| GET | `/v1/market/sales/categories` | SaleController::categories | **是** | ✅ |
| GET | `/v1/market/sales/detail` | SaleController::detail | 否 |  |
| GET | `/v1/market/sales/info` | SaleController::saleInfo | 否 | ✅ |
| GET | `/v1/market/sales/on` | SaleController::salesOn | 否 |  |
| GET | `/v1/market/sales/out` | SaleController::salesOut | 否 |  |
| GET | `/v1/market/stock/goodsLook` | PurchaseController::goodsLook | 否 |  |
| POST | `/v1/market/stock/import` | PurchaseController::import | 否 |  |
| GET | `/v1/market/stock/list` | PurchaseController::stockList | 否 |  |
| POST | `/v1/market/stock/look` | PurchaseController::stockLook | 否 |  |
| GET | `/v1/market/stock/lookall` | PurchaseController::lookAll | 否 | ✅ |
| GET | `/v1/market/stock/unlock` | PurchaseController::unlock | 否 |  |
| POST | `/v1/market/stock/wait` | PurchaseController::wait | 否 |  |
| GET | `/v1/open/appArchiveList` | OpenAppController::appArchiveList | 否 |  |
| GET | `/v1/open/appDetail` | OpenAppController::appDetail | 否 |  |
| GET | `/v1/open/appList` | OpenAppController::appList | 否 |  |
| GET | `/v1/open/authMyUserList` | OpenAppController::authMyUserList | 否 |  |
| GET | `/v1/open/authUrl` | OpenAppController::authUrl | 否 |  |
| POST | `/v1/open/authUser` | OpenAppController::authUser | 否 |  |
| GET | `/v1/open/authUserDetail` | OpenAppController::authUserDetail | 否 |  |
| GET | `/v1/open/authUserList` | OpenAppController::authUserList | 否 |  |
| POST | `/v1/open/deleteMyAuthUser` | OpenAppController::deleteMyAuthUser | 否 |  |
| POST | `/v1/open/lockGoods` | OpenAppController::lockGoods | 否 |  |
| POST | `/v1/open/unlockGoods` | OpenAppController::unlockGoods | 否 |  |
| GET | `/v1/open/userArchiveList` | OpenAppController::userArchiveList | 否 |  |
| GET | `/v1/open/userGoodsList` | OpenAppController::userGoodsList | 否 |  |
| GET | `/v1/payment/channel` | PaymentController::channel | 否 |  |
| GET | `/v1/search/hotLists` | SearchController::hotLists | **是** |  |
| POST | `/v1/security/resetOptPwd` | SecurityController::resetOptPwd | 否 |  |
| POST | `/v1/security/setOptPwd` | SecurityController::setOptPwd | 否 |  |
| POST | `/v1/security/updateOptPwd` | SecurityController::updateOptPwd | 否 |  |
| POST | `/v1/sms/reset` | SmsController::sendResetPasswordSms | 否 |  |
| POST | `/v1/sms/sendCode` | SmsController::sendCode | **是** |  |
| POST | `/v1/tao/bind` | TaoWalletController::taoBind | 否 |  |
| GET | `/v1/tao/orderlist` | TaoWalletController::orderList | 否 |  |
| POST | `/v1/tao/syncsingle` | TaoWalletController::syncSingle | 否 |  |
| POST | `/v1/tao/unbind` | TaoWalletController::unbind | 否 |  |
| GET | `/v1/ticket//cancel` | TicketWalletController::cancel | 否 |  |
| GET | `/v1/ticket//checklist` | TicketWalletController::checkList | 否 |  |
| GET | `/v1/ticket/amount` | TicketWalletController::amount | 否 |  |
| GET | `/v1/ticket/amountrecord` | TicketWalletController::amountRecord | 否 |  |
| POST | `/v1/ticket/bind` | TicketWalletController::bind | 否 |  |
| GET | `/v1/ticket/clearCache` | TicketWalletController::clearCache | 否 |  |
| POST | `/v1/ticket/getamount` | TicketWalletController::getAmount | 否 |  |
| GET | `/v1/ticket/syncamount` | TicketWalletController::syncAmount | 否 |  |
| POST | `/v1/ticket/syncsingle` | TicketWalletController::syncSingle | 否 |  |
| GET | `/v1/ticket/teaorderlist` | TicketWalletController::teaOrderList | 否 |  |
| POST | `/v1/ticket/unbind` | TicketWalletController::unbind | 否 |  |
| POST | `/v1/upload/image` | UploadController::image | 否 |  |
| GET | `/v1/user/address` | AddressController::index | 否 |  |
| POST | `/v1/user/address/create` | AddressController::create | 否 |  |
| GET | `/v1/user/address/default` | AddressController::addressDefault | 否 |  |
| GET | `/v1/user/address/delete` | AddressController::delete | 否 |  |
| POST | `/v1/user/address/update` | AddressController::update | 否 |  |
| POST | `/v1/user/bindMobile` | UserController::bindMobile | 否 |  |
| GET | `/v1/user/center` | UserController::center | 否 |  |
| POST | `/v1/user/changePassword` | UserController::changePassword | 否 |  |
| POST | `/v1/user/getMobileByMnp` | UserController::getMobileByMnp | 否 |  |
| GET | `/v1/user/info` | UserController::info | 否 | ✅ |
| GET | `/v1/user/message` | MessageController::index | 否 |  |
| GET | `/v1/user/message/detail` | MessageController::detail | 否 |  |
| POST | `/v1/user/message/setAllRead` | MessageController::setAllRead | 否 |  |
| POST | `/v1/user/message/setOneRead` | MessageController::setOneRead | 否 |  |
| GET | `/v1/user/notice/getNew` | NoticeController::getNew | **是** |  |
| GET | `/v1/user/real/info` | UserController::getRealInfo | 否 |  |
| POST | `/v1/user/real/verify` | UserController::real | 否 |  |
| POST | `/v1/user/resetPassword` | UserController::resetPassword | **是** |  |
| POST | `/v1/user/setInfo` | UserController::setInfo | 否 |  |
| GET | `/v1/user/unregisr/check` | UserController::unregisterCheck | 否 |  |
| POST | `/v1/user/unregister` | UserController::unregister | 否 |  |
| GET | `/v1/wallet/balance/query` | UserWalletController::balance | 否 |  |
| GET | `/v1/whitelist/check` | WhitelistController::check | 否 |  |
| GET | `/v1/whitelist/group` | WhitelistController::group | 否 |  |
| GET | `/v1/whitelist/permissions` | WhitelistController::permissions | 否 |  |
| POST | `/wallet/transfer` | WalletController::transfer | 否 |  |
| POST | `/wallet/withdraw` | WalletController::withdraw | 否 |  |

## 免登录接口清单（逐个核对用）

```
GET /v1/article/cate
GET /v1/article/detail
GET /v1/article/lists
GET /v1/common/captcha
GET /v1/common/config
GET /v1/common/protocol
GET /v1/index/bannerList
GET /v1/index/config
GET /v1/index/decorate
GET /v1/index/index
GET /v1/index/test
POST /v1/login/account
GET /v1/login/codeUrl
GET /v1/login/logout
POST /v1/login/mnpLogin
POST /v1/login/oaLogin
POST /v1/login/register
GET /v1/login/thirdAuth
GET /v1/login/thirdCheck
GET /v1/login/thirdLogin
GET /v1/market/purchase
GET /v1/market/sales/categories
GET /v1/search/hotLists
POST /v1/sms/sendCode
GET /v1/user/notice/getNew
POST /v1/user/resetPassword
```