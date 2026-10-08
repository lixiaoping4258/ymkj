# 已迁接口 · 请求/响应样例

> 本文由真实调用生成（服务运行于 `http://127.0.0.1:18000`）。
> **鉴权**：需要 token 的接口，把 token 放在名为 `token` 的请求头里（不是 `Authorization`）。

## 站点配置

```http
GET /v1/common/config
（免登录，无需 token）
```

```json
{"code":1,"show":0,"msg":"","data":{"buyOrSaleConfig":{"buy":{"high":{"is_open":"0","label":"最高","value":"200"},"low":{"is_open":"0","label":"最低","value":"5"},"multiple":{"is_open":"0","label":"整倍数","value":"0"}},"sale":{"high":{"is_open":"0","label":"最高","value":"150"},"low":{"is_open":"0","label":"最低","value":"1"},"multiple":{"is_open":"0","label":"整倍数","value":"5"}}},"outFee":6.66,"tradeSwitch":{"isOpen":0,"periods":[{"end":"11:30","start":"09:30"}],"tips":"兑换时段为上午09:30-11:30，下午13:30-15:30；周末及节假日暂停兑换！","todayType":"workday","todayTypeStr":"工作日","tradeSwitch":1}}}
```

## 交易时段配置

```http
GET /v1/common/trade/config
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"limitTime":1,"switch":0}}
```

## 用户资料

```http
GET /v1/user/info
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"avatar":"https://resourcetest.aicverse.com/user/03095f2b-2d26-4b57-84d8-488f72d8c404.jpg","create_time":1783678044,"has_opt_pwd":false,"has_password":true,"id":61089517,"is_real":false,"mobile":"18384123993","nickname":"丝雪","opt_pwd":null,"real_name":"","sex":0,"sn":178056289917980,"user_code":"","user_code_auth":{"tao_user_code":"","tea_user_code":""},"user_money":"0.00"}}
```

## 支付方式

```http
GET /v1/market/pay_way
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":[{"pay_name":"茶交所","pay_way":1},{"pay_name":"陶交所","pay_way":2}]}
```

## 兑换前置校验

```http
GET /v1/market/check/exchange
token: <32位token>
```

```json
{"code":0,"show":1,"msg":"当前不在兑换时间内，暂无法导入数字艺术品","data":[]}
```

## 兑换专区列表

```http
GET /v1/market/purchase?page_no=1&page_size=2
（免登录，无需 token）
```

```json
{"code":1,"show":0,"msg":"","data":{"count":0,"extend":[],"lists":[],"page_no":1,"page_size":10}}
```

## 秒转专区列表

```http
GET /v1/market/sales?page_no=1&page_size=2
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"count":0,"extend":[],"lists":[],"page_no":1,"page_size":2}}
```

## 秒转分类

```http
GET /v1/market/sales/categories
（免登录，无需 token）
```

```json
{"code":1,"show":0,"msg":"","data":[{"key":"all","title":"全部"}]}
```

## 秒转藏品封面

```http
GET /v1/market/sales/info?id=999999999
token: <32位token>
```

```json
{"code":0,"show":1,"msg":"未找到艺术品信息","data":[]}
```

## 库存汇总

```http
GET /v1/market/stock/lookall
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"num":0,"state":1}}
```

## 藏品封面

```http
GET /v1/market/purchase/info?id=178091946400112
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"collection_id":"116","id":178091946400112,"images":["https://collections-storage.tsl3060.com/attachment/uploads/202604/15/80eec0d9ecf76604bd410346c1fcdac6.png","https://collections-storage.tsl3060.com/attachment/uploads/202604/16/16f4e0475a27040b70e9107b35b94dcf.png"],"issuer":"元梦空间数字科技（成都）有限公司","name":"马上元梦 · 元梦启程","platform_id":1234554321,"platform_name":"元梦典藏"}}
```

## 计算总额

```http
GET /v1/market/purchase/showTotalAmount?app_id=1&amount=1&unit_price=100
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"totalAmount":"93.34"}}
```

## 兑换售出列表

```http
GET /v1/market/purchase/out?id=178056289901818
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"count":2,"extend":[],"lists":[{"amount":1,"archive_id":178091946400112,"available_amount":"0","create_time":"2026-09-14 13:57:07","grab_time":null,"id":178609302414145,"images":["https://collections-storage.tsl3060.com/attachment/uploads/202604/15/80eec0d9ecf76604bd410346c1fcdac6.png","https://collections-storage.tsl3060.com/attachment/uploads/202604/16/16f4e0475a27040b70e9107b35b94dcf.png"],"integral":"189.00","issuer":"元梦空间数字科技（成都）有限公司","name":"马上元梦 · 元梦启程","pay_way":1,"platform_name":"元梦典藏","receive_amount":1,"unit_price":"189.00"},{"amount":1,"archive_id":178091946400112,"available_amount":"0","create_time":"2026-09-09 15:58:25","grab_time":null,"id":178609302413490,"images":["https://collections-storage.tsl3060.com/attachment/uploads/202604/15/80eec0d9ecf76604bd410346c1fcdac6.png","https://collections-storage.tsl3060.com/attachment/uploads/202604 …（已截断）
```

## 兑换中列表

```http
GET /v1/market/purchase/on?id=178056289901818
token: <32位token>
```

```json
{"code":1,"show":0,"msg":"","data":{"count":0,"extend":[],"lists":[],"page_no":1,"page_size":25}}
```
