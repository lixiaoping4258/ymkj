# C 端 `app/api` 路由 × 免登录 × 迁移状态（由 scripts/gen-routes-php.ps1 生成）

> 静态解析原 PHP 项目：从路由文件按 **`Route::group` 分组** 解析 `Route::` 声明
> （**必须跟踪 group 前缀**，且**必须排除注释行**），
> 再从控制器抽 `$notNeedLogin` 交叉得出。

> **共 131 条有效路由，其中免登录 24 条，已迁 23 条。**

> ⚠️ **这是迁移时最容易标错的地方**，两个方向都会出事：
> 把该登录的标成公开 = 未授权访问；把该公开的标成需登录 = 前端匿名访问直接 403。
> **不要凭接口名猜。**

## 各路由文件的 group 前缀（路径由它决定）

| 文件 | 有效路由 | 被注释 | group 前缀 |
|---|---|---|---|
| `account.php` | 12 | 0 | `/v1/login` |
| `article.php` | 3 | 6 | `/v1/article` |
| `common.php` | 6 | 0 | `v1/common/` |
| `index.php` | 5 | 0 | `/v1/index` |
| `market.php` | 44 | 0 | `v1/market` |
| `open.php` | 13 | 0 | `v1/open/` |
| `payment.php` | 1 | 0 | `v1/payment` |
| `search.php` | 1 | 0 | `/v1/search` |
| `security.php` | 3 | 0 | `v1/security/` |
| `sms.php` | 2 | 0 | `/v1/sms` |
| `Tao.php` | 4 | 0 | `v1/tao` |
| `ticket.php` | 11 | 0 | `v1/ticket` |
| `upload.php` | 1 | 0 | `/v1/upload` |
| `user.php` | 21 | 0 | `/v1/user` |
| `wallet.php` | 1 | 0 | `v1/wallet` |
| `whitelist.php` | 9 | 0 | `/v1/whitelist``, ``trade/``, ``wallet/` |

## 全量路由表

| 方法 | 完整路径 | 控制器::方法 | 免登录 | 已迁 |
|---|---|---|---|---|
| GET | `/v1/article/about` | ArticleController::about | 否 | ✅ |
| GET | `/v1/article/detail` | ArticleController::detail | **是** |  |
| GET | `/v1/article/licenses` | ArticleController::license | 否 | ✅ |
| GET | `/v1/common/captcha` | CaptchaController::index | **是** | ✅ |
| GET | `/v1/common/config` | ConfigController::index | **是** | ✅ |
| GET | `/v1/common/notice` | NoticeController::index | 否 |  |
| GET | `/v1/common/platform/lists` | PlatformController::index | 否 | ✅ |
| GET | `/v1/common/protocol` | IndexController::policy | **是** | ✅ |
| GET | `/v1/common/trade/config` | IndexController::tradeConfig | 否 | ✅ |
| GET | `/v1/index/bannerList` | BannerController::getList | **是** | ✅ |
| GET | `/v1/index/config` | IndexController::config | **是** |  |
| GET | `/v1/index/decorate` | IndexController::decorate | **是** | ✅ |
| GET | `/v1/index/index` | IndexController::index | **是** | ✅ |
| GET | `/v1/index/test` | IndexController::test | **是** |  |
| POST | `/v1/login/account` | LoginController::account | **是** | ✅ |
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

## 免登录接口清单

```
GET /v1/article/detail
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
