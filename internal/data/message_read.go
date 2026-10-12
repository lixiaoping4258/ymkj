package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// messageReadRepo 对应 MessageLogic::setAllRead 的两条语句。
type messageReadRepo struct {
	db     *gorm.DB
	prefix string
}

func NewMessageReadRepo(db *gorm.DB, c *conf.Data) biz.MessageReadRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &messageReadRepo{db: db, prefix: prefix}
}

// unreadWhere 是两处共用的条件：
//
//	[['recv_id','=',$userId], ['is_read','=',0], ['is_del','=',0]]
//
// ⚠️ `is_del` 是业务删除标记（不是 delete_time 软删除）—— 原文就是用 is_del=0 过滤的。
//
//	收敛到一处，避免 count 与 update 的条件漂移。
func (r *messageReadRepo) unreadWhere(ctx context.Context, userID uint32) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"messages").
		Where("recv_id = ?", userID).
		Where("is_read = ?", 0).
		Where("is_del = ?", 0)
}

// CountUnread 对应 `Messages::where($where)->count()`。
func (r *messageReadRepo) CountUnread(ctx context.Context, userID uint32) (int64, error) {
	var n int64
	err := r.unreadWhere(ctx, userID).Count(&n).Error
	return n, err
}

// MarkAllRead 对应 `Messages::where($where)->update(['is_read' => 1])`。
//
// ⚠️ 只更新 is_read 一列。原文的 update() 不会碰 update_time
//
//	（除非模型开了自动时间戳；Messages 的模型未声明）。
func (r *messageReadRepo) MarkAllRead(ctx context.Context, userID uint32) error {
	return r.unreadWhere(ctx, userID).Update("is_read", 1).Error
}

/* ------------------------------------------------- setOneRead */

// oneWhere 是 setOneRead 的条件（与 setAllRead 的 unreadWhere 不同，注意区分）：
//
//	[['message_id','=',$id], ['recv_id','=',$userId], ['is_del','=',0]]
//
// ⚠️ 这里过滤的是 **message_id**，不是 is_read —— setOneRead 要先查归属再决定
//
//	要不要更新，所以查询条件里不能带 is_read。
func (r *messageReadRepo) oneWhere(ctx context.Context, userID uint32, messageID int64) *gorm.DB {
	return r.db.WithContext(ctx).
		Table(r.prefix+"messages").
		Where("message_id = ?", messageID).
		Where("recv_id = ?", userID).
		Where("is_del = ?", 0)
}

// FindByMessageID 对应 `Messages::where($where)->findOrEmpty()`（只取 is_read）。
func (r *messageReadRepo) FindByMessageID(
	ctx context.Context, userID uint32, messageID int64,
) (*biz.MessageReadState, bool, error) {
	var row struct {
		IsRead int32 `gorm:"column:is_read"`
	}
	err := r.oneWhere(ctx, userID, messageID).Select("is_read").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.MessageReadState{IsRead: row.IsRead}, true, nil
}

// MarkOneRead 对应 `Messages::where($where)->update(['is_read' => 1])`。
func (r *messageReadRepo) MarkOneRead(ctx context.Context, userID uint32, messageID int64) error {
	return r.oneWhere(ctx, userID, messageID).Update("is_read", 1).Error
}
