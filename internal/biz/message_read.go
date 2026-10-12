package biz

import "context"

// MessageReadUsecase 对应 app/api/logic/user/MessageLogic.php 的 setAllRead。
//
// ⚠️ 本文件只实现 setAllRead。MessageLogic 里还有 setOneRead / detail /
// isNotReadCount / setNoticeData 未迁（见 docs/RECON-user-message.md）。
type MessageReadUsecase struct {
	repo MessageReadRepo
}

func NewMessageReadUsecase(repo MessageReadRepo) *MessageReadUsecase {
	return &MessageReadUsecase{repo: repo}
}

// MessageReadRepo 消息已读状态。
type MessageReadRepo interface {
	// CountUnread 对应：
	//   Messages::where([['recv_id','=',$userId],['is_read','=',0],['is_del','=',0]])->count()
	CountUnread(ctx context.Context, userID uint32) (int64, error)
	// MarkAllRead 对应同一 where 下的 `->update(['is_read' => 1])`。
	MarkAllRead(ctx context.Context, userID uint32) error

	// ---- setOneRead 用到的两个 ----
	// FindByMessageID 对应：
	//   Messages::where([['message_id','=',$id],['recv_id','=',$userId],['is_del','=',0]])->findOrEmpty()
	//
	// ⚠️ 传进来的 id 匹配的是 **message_id** 列，不是主键 id。
	// ⚠️ 带 recv_id -> 越权防护。
	FindByMessageID(ctx context.Context, userID uint32, messageID int64) (*MessageReadState, bool, error)
	// MarkOneRead 对应同一 where 下的 update(['is_read' => 1])
	MarkOneRead(ctx context.Context, userID uint32, messageID int64) error
}

// MessageReadState 只需要 is_read 一列。
type MessageReadState struct {
	IsRead int32
}

// ErrMessageNotFound 对应 `self::setError('消息不存在或无权操作')`。
var ErrMessageNotFound = errMessageNotFound{}

type errMessageNotFound struct{}

func (errMessageNotFound) Error() string { return "消息不存在或无权操作" }

// SetOneRead 逐字对应 MessageLogic::setOneRead($userId, $params)。
//
// 原文：
//
//	$messageId = $params['id'] ?? 0;
//	$where = [['message_id','=',$messageId], ['recv_id','=',$userId], ['is_del','=',0]];
//	$message = Messages::where($where)->findOrEmpty();
//	if ($message->isEmpty()) { self::setError('消息不存在或无权操作'); return false; }
//	if ($message['is_read'] == 1) { return true; }               // ⚠️ 已读 -> 短路，不 UPDATE
//	$result = Messages::where($where)->update(['is_read' => 1]);
//	if ($result === false) { self::setError('标记已读有误'); return false; }
//	return true;
//
// ⚠️ 两个短路/防护必须照搬：
//  1. 先查归属（recv_id + is_del=0）—— 查不到就报"消息不存在或无权操作"，
//     **不能先更新再看 affected rows**，否则别人的消息会被误标。
//  2. 已经是已读时**不发 UPDATE**（可观测差异：affected rows / 慢查询日志）。
func (uc *MessageReadUsecase) SetOneRead(ctx context.Context, userID uint32, messageID int64) error {
	row, found, err := uc.repo.FindByMessageID(ctx, userID, messageID)
	if err != nil {
		return err
	}
	if !found || row == nil {
		return ErrMessageNotFound
	}
	if row.IsRead == 1 {
		return nil // 已是已读，直接成功
	}
	return uc.repo.MarkOneRead(ctx, userID, messageID)
}

// SetAllRead 逐字对应 MessageLogic::setAllRead($userId)。
//
// 原文：
//
//	$where = [['recv_id','=',$userId], ['is_read','=',0], ['is_del','=',0]];
//	$count = Messages::where($where)->count();
//	if (empty($count)) { return true; }                       // ⚠️ 没有未读 -> 直接成功，不执行 UPDATE
//	$result = Messages::where($where)->update(['is_read' => 1]);
//	if ($result === false) { self::setError('标记已读有误'); return false; }
//	return true;
//	// catch: self::setError('标记已读失败：' . $e->getMessage());
//
// ⚠️ `empty($count)` 对 0 为真 —— 所以"没有未读消息"时**不会**发出 UPDATE 语句。
//
//	这是一个可观测的差异（DB 的 affected rows / 慢查询日志），照搬这个短路。
//
// ⚠️ `empty()` 对字符串 "0" 也为真，但 count() 返回的是 int，这里无影响。
func (uc *MessageReadUsecase) SetAllRead(ctx context.Context, userID uint32) error {
	n, err := uc.repo.CountUnread(ctx, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		// 对应 `if (empty($count)) { return true; }`
		return nil
	}
	return uc.repo.MarkAllRead(ctx, userID)
}

// ErrMarkReadFailed 对应 `self::setError('标记已读有误')`。
var ErrMarkReadFailed = errMarkReadFailed{}

type errMarkReadFailed struct{}

func (errMarkReadFailed) Error() string { return "标记已读有误" }

// MarkReadFailedMsg 对应 catch 分支：'标记已读失败：' . $e->getMessage()
func MarkReadFailedMsg(cause string) string { return "标记已读失败：" + cause }
