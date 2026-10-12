package biz

import "context"

// MessageDetailUsecase 对应 app/api/logic/user/MessageLogic.php 的 detail。
type MessageDetailUsecase struct {
	repo MessageDetailRepo
}

func NewMessageDetailUsecase(repo MessageDetailRepo) *MessageDetailUsecase {
	return &MessageDetailUsecase{repo: repo}
}

// MessageDetailRepo 消息详情（两表 INNER JOIN）。
type MessageDetailRepo interface {
	// FindDetail 对应：
	//
	//	Messages::alias('m')->join('messages_body mb', 'm.message_id = mb.id')
	//	    ->where([['m.message_id','=',$id], ['m.recv_id','=',$userId], ['m.is_del','=',0]])
	//	    ->field([...10 列...])->findOrEmpty();
	FindDetail(ctx context.Context, userID uint32, messageID int64) (*MessageDetailRow, bool, error)

	// MarkReadByPrimaryKey 对应原文的自动标已读：
	//
	//	Messages::where('id', $messageId)->update(['is_read' => 1]);
	//
	// ⚠️⚠️ **这里传的是主键 id，而 $messageId 在查询里是当 message_id 用的**。
	//    详见 MessageDetailUsecase.Detail 的说明。
	MarkReadByPrimaryKey(ctx context.Context, primaryKey int64) error
}

// MessageDetailRow 对应 field() 里的 10 列。
type MessageDetailRow struct {
	// x_messages 的列
	ID        int64
	SendID    *int64
	RecvID    *int64
	MessageID *int64
	IsRead    int32
	// ⚠️ 这里**没有**像 MessageLists 那样把 create_time 重新格式化 ——
	//    原文 detail() 直接 toArray()，所以是什么类型就输出什么类型。
	CreateTime string
	// x_messages_body 的列
	Type  int32
	Title string
	Body  string
	Flag  string
}

// MessageDetailResult 是对外形态（10 列 + 计算列 type_name）。
type MessageDetailResult struct {
	Row      MessageDetailRow
	TypeName string
}

// ErrMessageDetailNotFound 对应 `self::setError('消息不存在或无权查看')`。
var ErrMessageDetailNotFound = errMsgDetailNotFound{}

type errMsgDetailNotFound struct{}

func (errMsgDetailNotFound) Error() string { return "消息不存在或无权查看" }

// GetTypeDesc 逐字对应 app\common\enum\message\TypeEnum::getTypeDesc($value)。
//
//	$data = [0 => '点对点消息', 1 => '广播消息'];
//	if ($value === true) { return $data; }        // 传 true 返回整张表（Go 侧用单独函数）
//	return $data[$value] ?? '';                    // ⚠️ 未知值返回**空串**，不是 null
func GetTypeDesc(value int32) string {
	switch value {
	case TypeEnumPoint:
		return "点对点消息"
	case TypeEnumNotice:
		return "广播消息"
	default:
		return "" // ⚠️ 未知值 -> 空串
	}
}

// 逐字对应 TypeEnum 的两个常量
const (
	TypeEnumPoint  int32 = 0 // 点对点消息
	TypeEnumNotice int32 = 1 // 广播消息
)

// Detail 逐字对应 MessageLogic::detail($userId, $params)。
//
// 原文：
//
//	$messageId = $params['id'] ?? 0;
//	$where = [['m.message_id','=',$messageId], ['m.recv_id','=',$userId], ['m.is_del','=',0]];
//	$message = Messages::alias('m')->join('messages_body mb','m.message_id = mb.id')
//	    ->where($where)->field([...])->findOrEmpty();
//	if ($message->isEmpty()) { self::setError('消息不存在或无权查看'); return false; }
//	$result = $message->toArray();
//
//	// 自动标记为已读
//	if ($result['is_read'] == 0) {
//	    Messages::where('id', $messageId)->update(['is_read' => 1]);   // 🔴 见下
//	    $result['is_read'] = 1;
//	}
//	$result['type_name'] = TypeEnum::getTypeDesc($result['type']);
//	return $result;
//
// 🔴🔴 **原实现里有一个真 bug，本实现刻意照搬**：
//
//	查询条件用的是 `m.message_id = $messageId`（按**业务字段** message_id 查），
//	但自动标已读的 UPDATE 用的是 `where('id', $messageId)`（按**主键** id 更新）。
//	同一个 `$messageId` 被当成两种不同的语义使用。
//
//	后果：UPDATE 会打到 `x_messages.id = <message_id 的值>` 那一行 ——
//	**很可能是别的用户的消息，或者根本没有这一行**。
//	也就是说"打开详情自动标已读"**很可能没生效**（甚至可能误标别人的消息）。
//
//	按"重构时全部按照原项目逻辑、不要改动原来的代码逻辑"的要求**照搬**这个行为，
//	不改成本该正确的 `where('message_id', ...)`。
//	已在 README 的原系统缺陷一节记录。
func (uc *MessageDetailUsecase) Detail(
	ctx context.Context, userID uint32, messageID int64,
) (*MessageDetailResult, error) {
	row, found, err := uc.repo.FindDetail(ctx, userID, messageID)
	if err != nil {
		return nil, err
	}
	if !found || row == nil {
		return nil, ErrMessageDetailNotFound
	}

	// 自动标记为已读 —— ⚠️ 刻意按原文用**主键**更新（见上方说明）
	if row.IsRead == 0 {
		if err := uc.repo.MarkReadByPrimaryKey(ctx, messageID); err != nil {
			return nil, err
		}
		// 原文在这一步之后把返回值里的 is_read 改成 1（**只是响应里的值**，
		// 与 DB 是否真的更新成功无关）
		row.IsRead = 1
	}

	return &MessageDetailResult{Row: *row, TypeName: GetTypeDesc(row.Type)}, nil
}
