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
