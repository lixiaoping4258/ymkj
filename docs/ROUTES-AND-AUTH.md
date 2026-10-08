# C 端 `app/api` 路由 × 免登录 × 迁移状态

> 由静态解析生成：从 16 个路由文件抽 Route:: 声明，从 33 个控制器抽 \ 数组，交叉得出。
> **共 141 条路由，其中免登录 26 条，已迁 14 条。**

> ⚠️ **这是迁移时最容易标错的地方**，而且两个方向都会出事：
> 把该登录的标成公开 = 未授权访问；把该公开的标成需登录 = 前端匿名访问直接 403。
> **不要凭接口名猜** —— 这张表是唯一依据。改 proto 的 service/method 名时要同步改 `internal/server/middleware.go` 的 `defaultPublicOps`。

| 方法 | 完整路径 | 控制器::方法 | 免登录 | 已迁 |
|---|---|---|---|---|
| POST | `/account/account` | LoginController::account | **是** |  |
| GET | `/account/codeUrl` | LoginController::codeUrl | **是** |  |
| GET | `/account/logout` | LoginController::logout | **是** |  |
| POST | `/account/mnpAuthBind` | LoginController::mnpAuthBind | 否 |  |
| POST | `/account/mnpLogin` | LoginController::mnpLogin | **是** |  |
| POST | `/account/oaAuthBind` | LoginController::oaAuthBind | 否 |  |
| POST | `/account/oaLogin` | LoginController::oaLogin | **是** |  |
| POST | `/account/register` | LoginController::register | **是** |  |
| GET | `/account/thirdAuth` | LoginController::thirdAuth | **是** |  |
| GET | `/account/thirdCheck` | LoginController::thirdCheck | **是** |  |
| GET | `/account/thirdLogin` | LoginController::thirdLogin | **是** |  |
| POST | `/account/updateUser` | LoginController::updateUser | 否 |  |
| GET | `/article/about` | ArticleController::about | 否 |  |
| GET | `/article/addCollect` | ArticleController::addCollect | 否 |  |
| GET | `/article/cancelCollect` | ArticleController::cancelCollect | 否 |  |
| GET | `/article/cate` | ArticleController::cate | **是** |  |
| GET | `/article/collect` | ArticleController::collect | 否 |  |
| GET | `/article/detail` | ArticleController::detail | **是** |  |
| GET | `/article/licenses` | ArticleController::license | 否 |  |
| GET | `/article/lists` | ArticleController::lists | **是** |  |
| GET | `/common/captcha` | CaptchaController::index | **是** |  |
| GET | `/common/config` | ConfigController::index | **是** | ✅ |
| GET | `/common/notice` | NoticeController::index | 否 |  |
| GET | `/common/platform/lists` | PlatformController::index | 否 |  |
| GET | `/common/protocol` | IndexController::policy | **是** |  |
| GET | `/common/trade/config` | IndexController::tradeConfig | 否 | ✅ |
| GET | `/index/bannerList` | BannerController::getList | **是** |  |
| GET | `/index/config` | IndexController::config | **是** |  |
| GET | `/index/decorate` | IndexController::decorate | **是** |  |
| GET | `/index/index` | IndexController::index | **是** |  |
| GET | `/index/test` | IndexController::test | **是** |  |
| GET | `/market/check/exchange` | PurchaseController::checkExchange | 否 | ✅ |
| POST | `/market/order/create/purchase` | OrderController::createPurchaseOrder | 否 |  |
| GET | `/market/orders/create/query` | OrderController::queryCreateOrderStatus | 否 |  |
| POST | `/market/orders/create/sale` | OrderController::createSaleOrder | 否 |  |
| POST | `/market/orders/purchase/delete` | PurchaseOrdersController::delete | 否 |  |
| GET | `/market/orders/purchase/detail` | PurchaseOrdersController::orderDetail | 否 |  |
| GET | `/market/orders/purchase/lists` | PurchaseOrdersController::orderLists | 否 |  |
| GET | `/market/orders/purchase/my_lists` | PurchaseOrdersController::myLists | 否 |  |
| POST | `/market/orders/purchase/offline` | PurchaseOrdersController::offline | 否 |  |
| GET | `/market/orders/purchase/saledetail` | PurchaseOrdersController::saleDetail | 否 |  |
| GET | `/market/orders/purchase/salelist` | PurchaseOrdersController::saleList | 否 |  |
| GET | `/market/orders/sale/cancel` | SaleOrdersController::cancelOrder | 否 |  |
| GET | `/market/orders/sale/detail` | SaleOrdersController::detail | 否 |  |
| GET | `/market/orders/sale/goodslist` | SaleOrdersController::goodsList | 否 |  |
| GET | `/market/orders/sale/lists` | SaleOrdersController::orderLists | 否 |  |
| GET | `/market/orders/sale/recorddetail` | SaleOrdersController::recordDetail | 否 |  |
| GET | `/market/orders/sale/saleslist` | SaleOrdersController::salesList | 否 |  |
| GET | `/market/pay_way` | PurchaseController::payWay | 否 | ✅ |
| GET | `/market/price_range` | PurchaseController::priceRange | 否 |  |
| GET | `/market/purchase` | PurchaseController::index | **是** | ✅ |
| POST | `/market/purchase/create` | PurchaseController::create | 否 |  |
| GET | `/market/purchase/detail` | PurchaseController::detail | 否 |  |
| POST | `/market/purchase/grabPriceCheck` | PurchaseController::grabPriceCheck | 否 |  |
| POST | `/market/purchase/grabPriceLook` | PurchaseController::grabPriceLook | 否 |  |
| POST | `/market/purchase/grabPriceSubmit` | PurchaseController::grabPriceSubmit | 否 |  |
| GET | `/market/purchase/info` | PurchaseController::purchaseInfo | 否 | ✅ |
| GET | `/market/purchase/on` | PurchaseController::salesOn | 否 | ✅ |
| GET | `/market/purchase/out` | PurchaseController::salesOut | 否 | ✅ |
| POST | `/market/purchase/purchaseBuy` | PurchaseController::purchaseBuy | 否 |  |
| GET | `/market/purchase/showTotalAmount` | PurchaseOrdersController::showTotalAmount | 否 | ✅ |
| GET | `/market/sales` | SaleController::index | 否 | ✅ |
| POST | `/market/sales/buy` | SaleController::buy | 否 |  |
| GET | `/market/sales/categories` | SaleController::categories | **是** | ✅ |
| GET | `/market/sales/detail` | SaleController::detail | 否 |  |
| GET | `/market/sales/info` | SaleController::saleInfo | 否 | ✅ |
| GET | `/market/sales/on` | SaleController::salesOn | 否 |  |
| GET | `/market/sales/out` | SaleController::salesOut | 否 |  |
| GET | `/market/stock/goodsLook` | PurchaseController::goodsLook | 否 |  |
| POST | `/market/stock/import` | PurchaseController::import | 否 |  |
| GET | `/market/stock/list` | PurchaseController::stockList | 否 |  |
| POST | `/market/stock/look` | PurchaseController::stockLook | 否 |  |
| GET | `/market/stock/lookall` | PurchaseController::lookAll | 否 | ✅ |
| GET | `/market/stock/unlock` | PurchaseController::unlock | 否 |  |
| POST | `/market/stock/wait` | PurchaseController::wait | 否 |  |
| GET | `/open/appArchiveList` | OpenAppController::appArchiveList | 否 |  |
| GET | `/open/appDetail` | OpenAppController::appDetail | 否 |  |
| GET | `/open/appList` | OpenAppController::appList | 否 |  |
| GET | `/open/authMyUserList` | OpenAppController::authMyUserList | 否 |  |
| GET | `/open/authUrl` | OpenAppController::authUrl | 否 |  |
| POST | `/open/authUser` | OpenAppController::authUser | 否 |  |
| GET | `/open/authUserDetail` | OpenAppController::authUserDetail | 否 |  |
| GET | `/open/authUserList` | OpenAppController::authUserList | 否 |  |
| POST | `/open/deleteMyAuthUser` | OpenAppController::deleteMyAuthUser | 否 |  |
| POST | `/open/lockGoods` | OpenAppController::lockGoods | 否 |  |
| POST | `/open/unlockGoods` | OpenAppController::unlockGoods | 否 |  |
| GET | `/open/userArchiveList` | OpenAppController::userArchiveList | 否 |  |
| GET | `/open/userGoodsList` | OpenAppController::userGoodsList | 否 |  |
| GET | `/payment/channel` | PaymentController::channel | 否 |  |
| GET | `/search/hotLists` | SearchController::hotLists | **是** |  |
| POST | `/security/resetOptPwd` | SecurityController::resetOptPwd | 否 |  |
| POST | `/security/setOptPwd` | SecurityController::setOptPwd | 否 |  |
| POST | `/security/updateOptPwd` | SecurityController::updateOptPwd | 否 |  |
| POST | `/sms/reset` | SmsController::sendResetPasswordSms | 否 |  |
| POST | `/sms/sendCode` | SmsController::sendCode | **是** |  |
| POST | `/tao/bind` | TaoWalletController::taoBind | 否 |  |
| GET | `/tao/orderlist` | TaoWalletController::orderList | 否 |  |
| POST | `/tao/syncsingle` | TaoWalletController::syncSingle | 否 |  |
| POST | `/tao/unbind` | TaoWalletController::unbind | 否 |  |
| GET | `/ticket//cancel` | TicketWalletController::cancel | 否 |  |
| GET | `/ticket//checklist` | TicketWalletController::checkList | 否 |  |
| GET | `/ticket/amount` | TicketWalletController::amount | 否 |  |
| GET | `/ticket/amountrecord` | TicketWalletController::amountRecord | 否 |  |
| POST | `/ticket/bind` | TicketWalletController::bind | 否 |  |
| GET | `/ticket/clearCache` | TicketWalletController::clearCache | 否 |  |
| POST | `/ticket/getamount` | TicketWalletController::getAmount | 否 |  |
| GET | `/ticket/syncamount` | TicketWalletController::syncAmount | 否 |  |
| POST | `/ticket/syncsingle` | TicketWalletController::syncSingle | 否 |  |
| GET | `/ticket/teaorderlist` | TicketWalletController::teaOrderList | 否 |  |
| POST | `/ticket/unbind` | TicketWalletController::unbind | 否 |  |
| POST | `/upload/image` | UploadController::image | 否 |  |
| GET | `/user/address` | AddressController::index | 否 |  |
| POST | `/user/address/create` | AddressController::create | 否 |  |
| GET | `/user/address/default` | AddressController::addressDefault | 否 |  |
| GET | `/user/address/delete` | AddressController::delete | 否 |  |
| POST | `/user/address/update` | AddressController::update | 否 |  |
| POST | `/user/bindMobile` | UserController::bindMobile | 否 |  |
| GET | `/user/center` | UserController::center | 否 |  |
| POST | `/user/changePassword` | UserController::changePassword | 否 |  |
| POST | `/user/getMobileByMnp` | UserController::getMobileByMnp | 否 |  |
| GET | `/user/info` | UserController::info | 否 | ✅ |
| GET | `/user/message` | MessageController::index | 否 |  |
| GET | `/user/message/detail` | MessageController::detail | 否 |  |
| POST | `/user/message/setAllRead` | MessageController::setAllRead | 否 |  |
| POST | `/user/message/setOneRead` | MessageController::setOneRead | 否 |  |
| GET | `/user/notice/getNew` | NoticeController::getNew | **是** |  |
| GET | `/user/real/info` | UserController::getRealInfo | 否 |  |
| POST | `/user/real/verify` | UserController::real | 否 |  |
| POST | `/user/resetPassword` | UserController::resetPassword | **是** |  |
| POST | `/user/setInfo` | UserController::setInfo | 否 |  |
| GET | `/user/unregisr/check` | UserController::unregisterCheck | 否 |  |
| POST | `/user/unregister` | UserController::unregister | 否 |  |
| GET | `/wallet/balance/query` | UserWalletController::balance | 否 |  |
| POST | `/whitelist/buy` | TradeController::buy | 否 |  |
| GET | `/whitelist/check` | WhitelistController::check | 否 |  |
| GET | `/whitelist/group` | WhitelistController::group | 否 |  |
| GET | `/whitelist/permissions` | WhitelistController::permissions | 否 |  |
| POST | `/whitelist/sell` | TradeController::sell | 否 |  |
| POST | `/whitelist/trade/buy` | TradeController::buy | 否 |  |
| POST | `/whitelist/transfer` | WalletController::transfer | 否 |  |
| POST | `/whitelist/withdraw` | WalletController::withdraw | 否 |  |

## 免登录接口清单（27 条，逐个核对用）

```
POST /account/account
GET /account/codeUrl
GET /account/logout
POST /account/mnpLogin
POST /account/oaLogin
POST /account/register
GET /account/thirdAuth
GET /account/thirdCheck
GET /account/thirdLogin
GET /article/cate
GET /article/detail
GET /article/lists
GET /common/captcha
GET /common/config
GET /common/protocol
GET /index/bannerList
GET /index/config
GET /index/decorate
GET /index/index
GET /index/test
GET /market/purchase
GET /market/sales/categories
GET /search/hotLists
POST /sms/sendCode
GET /user/notice/getNew
POST /user/resetPassword
```