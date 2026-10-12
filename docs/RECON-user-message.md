# 待迁侦察：`/v1/user/message`（消息列表）

> 静态分析结果，**未实现**。这个接口有 3 个不读源码绝对想不到的性质。

## 路由与鉴权

- 路径：**`GET /v1/user/message`**（group `/v1/user`）
- 控制器：`app/api/controller/v1/user/MessageController.php::index`
- **需登录**（`MessageController` 未声明 `$notNeedLogin`）

## 控制器

```php
public function index(): Json
{
    // 添加广播消息
    MessageLogic::setNoticeData($this->userId);      // 🔴 写操作！
    // 获取消息列表
    return $this->dataLists(new MessageLists());
}
```

## 🔴 性质 1：这是"读接口里藏写操作"

`setNoticeData($userId)`（`app/api/logic/user/MessageLogic.php`）每次请求都会跑：

```php
public static function setNoticeData(int $userId): bool
{
    // 查询所有未删除的广播消息（type=1）
    $broadcastMessages = MessagesBody::where('type', TypeEnum::TYPE_NOTICE)
        ->whereNull('delete_time')->select()->toArray();
    if (empty($broadcastMessages)) { return true; }

    // 获取该用户已存在的消息ID列表
    $existingMessageIds = Messages::where('recv_id', $userId)->column('message_id');

    $addAll = [];
    foreach ($broadcastMessages as $messageBody) {
        if (in_array($messageBody['id'], $existingMessageIds)) { continue; }   // 已有则跳过
        $addAll[] = [
            'id'          => Id::gen(),                     // ⚠️ 用共享的 Redis ID 生成器
            'send_id'     => $messageBody['create_admin_id'] ?? 0,
            'recv_id'     => $userId,
            'message_id'  => $messageBody['id'],
            'create_time' => TimeHelper::now(),
        ];
    }
    if (!empty($addAll)) { return Messages::insertAll($addAll); }              // 批量插入
    return true;
}
```

**含义**：用户第一次打开消息列表时，系统会把所有他还没收到的**广播消息**补插进来。
所以这个"查询"接口**会写 `x_messages` 表**，且写的是**批量插入**。

迁移时必须：
- 给它写权限
- 考虑并发（两个请求同时进来会重复插入，因为 `in_array` 检查与插入之间没有锁）
- `Id::gen()` 用的是**与 PHP 共用的 Redis ID 生成器**（已迁，见 `data/idgen.go`）

## 🔴 性质 2：`is_read` / `type` 过滤是**死代码**

`MessageLists` 的构造函数：

```php
public function __construct($params = [])
{
    parent::__construct();
    $this->params['is_read'] = $params['is_read'] ?? -1;
    $this->params['type']    = $params['type']    ?? -1;
}
private function queryWhere(): array
{
    $where = [['m.recv_id','=',$this->userId], ['m.is_del','=',0]];
    if ($this->params['is_read'] >= 0) { $where[] = ['m.is_read','=',$this->params['is_read']]; }
    if ($this->params['type']    >= 0) { $where[] = ['m.type','=',$this->params['type']]; }
    return $where;
}
```

而控制器是 **`new MessageLists()` —— 没有传任何参数**：

```php
return $this->dataLists(new MessageLists());
```

于是 `$params = []` → `is_read = -1`、`type = -1` → **两个过滤条件永远不生效**。

**迁移时不要"顺手"把这两个参数接上** —— 那会改变行为（虽然看起来是修 bug）。
按"不改动原项目逻辑"照搬：**不实现这两个过滤**。

## 🔴 性质 3：两表 INNER JOIN

```php
$sortRaw = 'm.create_time desc';
$fields = [
    'mb.id', 'mb.type', 'mb.title', 'mb.body', 'mb.create_time',
    'mb.flag', 'm.send_id', 'm.recv_id', 'm.is_read',
];
$lists = Messages::alias('m')
    ->join('messages_body mb', 'm.message_id = mb.id')      // ⚠️ INNER JOIN，不是 LEFT
    ->where($where)->field($fields)->order($sortRaw)
    ->limit($this->limitOffset, $this->limitLength)->select();

$result = $lists->toArray();
foreach ($result as &$item) {
    $item['create_time'] = date('Y-m-d H:i:s', strtotime($item['create_time']));  // 重新格式化
    $item['type_name']   = TypeEnum::getTypeDesc($item['type']);                  // 计算列
}
return $result;

public function count(): int {
    return Messages::alias('m')
        ->join('messages_body mb', 'm.message_id = mb.id')      // ⚠️ 同一个 JOIN
        ->where($this->queryWhere())->count();
}
```

- **`join()` 在 ThinkPHP 里默认是 INNER JOIN**，所以 `m.message_id` 在 `x_messages_body`
  里不存在的消息**不会出现**，`count()` 也不含它们 —— 与 `/v1/user/address` 的
  `hasWhere` 是同一类问题
- **`create_time` 被重新格式化**：`date('Y-m-d H:i:s', strtotime($item['create_time']))`
  —— 如果 `mb.create_time` 是 datetime，这一步是幂等的；如果是 int 时间戳，
  则会变成字符串。**实现前必须确认该列类型**
- **`type_name` 是计算列**，不在 SQL 里

`TypeEnum::getTypeDesc($value)`：

```php
$data = [0 => '点对点消息', 1 => '广播消息'];
if ($value === true) { return $data; }
return $data[$value] ?? '';          // ⚠️ 未知值返回**空串**，不是 null
```

## 对外形态（11 个键）

```
mb.id, mb.type, mb.title, mb.body, mb.create_time, mb.flag,   <- 来自 x_messages_body
m.send_id, m.recv_id, m.is_read,                               <- 来自 x_messages
type_name                                                      <- 计算列
create_time                                                    <- 被重新格式化
```

> ⚠️ 两个来源表的列名**没有加表前缀**（ThinkPHP 的 `field('mb.id')` 输出键就是 `id`），
> 所以 `id` 是 `x_messages_body.id`，而 `send_id`/`recv_id`/`is_read` 是 `x_messages` 的列。

## 实现前需要确认

| # | 项 |
|---|---|
| 1 | `mb.create_time`（`x_messages_body.create_time`）是 **datetime 还是 int** —— 决定要不要重新格式化 |
| 2 | `x_messages` 与 `x_messages_body` 的完整列结构 |
| 3 | `x_messages.is_del` 的意义（`=0` 过滤），是否有对应的软删除列 |
| 4 | `type` 字段在 `x_messages` 还是 `x_messages_body` 里（`field` 取的是 `mb.type`，但过滤用的 `m.type` —— **两个表的 type 可能不是一回事，需要核实**） |

> ⚠️ 第 4 条尤其可疑：`queryWhere` 过滤的是 `m.type`，而 `lists` 取的是 `mb.type`。
> 如果这两列语义不同，那么"过滤"和"取值"取的就不是同一个东西 —— 但因为
> 过滤是死代码（性质 2），当前不会暴露。

## 为什么这轮没实现

两表 INNER JOIN + 读时写库 + 计算列 + 3 个待确认项，而本次会话上下文预算已接近耗尽。
在预算耗尽时写复杂查询的风险是产出**无法验证**的实现（本会话已发生过多次返工）。
先把读到的信息固定下来。
