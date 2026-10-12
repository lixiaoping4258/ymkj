package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// messageDetailRepo 对应 MessageLogic::detail 的两条语句。
type messageDetailRepo struct {
	db     *gorm.DB
	prefix string
}

func NewMessageDetailRepo(db *gorm.DB, c *conf.Data) biz.MessageDetailRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &messageDetailRepo{db: db, prefix: prefix}
}

// FindDetail 逐字对应：
//
//	Messages::alias('m')->join('messages_body mb', 'm.message_id = mb.id')
//	    ->where([['m.message_id','=',$id], ['m.recv_id','=',$userId], ['m.is_del','=',0]])
//	    ->field(['m.id','m.send_id','m.recv_id','m.message_id','m.is_read','m.create_time',
//	             'mb.type','mb.title','mb.body','mb.flag'])
//	    ->findOrEmpty();
//
// ⚠️ INNER JOIN（ThinkPHP 的 join() 默认）—— 关联不上的消息查不到。
// ⚠️ 带 recv_id -> 越权防护。
func (r *messageDetailRepo) FindDetail(
	ctx context.Context, userID uint32, messageID int64,
) (*biz.MessageDetailRow, bool, error) {
	var row struct {
		ID         int64     `gorm:"column:id"`
		SendID     *int64    `gorm:"column:send_id"`
		RecvID     *int64    `gorm:"column:recv_id"`
		MessageID  *int64    `gorm:"column:message_id"`
		IsRead     int32     `gorm:"column:is_read"`
		CreateTime time.Time `gorm:"column:create_time"`
		Type       int32     `gorm:"column:type"`
		Title      string    `gorm:"column:title"`
		Body       string    `gorm:"column:body"`
		Flag       string    `gorm:"column:flag"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"messages AS m").
		Joins("INNER JOIN "+r.prefix+"messages_body AS mb ON m.message_id = mb.id").
		Select("m.id AS id, m.send_id AS send_id, m.recv_id AS recv_id, "+
			"m.message_id AS message_id, m.is_read AS is_read, m.create_time AS create_time, "+
			"mb.type AS type, mb.title AS title, mb.body AS body, mb.flag AS flag").
		Where("m.message_id = ?", messageID).
		Where("m.recv_id = ?", userID).
		Where("m.is_del = ?", 0).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.MessageDetailRow{
		ID: row.ID, SendID: row.SendID, RecvID: row.RecvID, MessageID: row.MessageID,
		IsRead: row.IsRead, CreateTime: row.CreateTime.Format("2006-01-02 15:04:05"),
		Type: row.Type, Title: row.Title, Body: row.Body, Flag: row.Flag,
	}, true, nil
}

// MarkReadByPrimaryKey 逐字对应**原文的** `Messages::where('id', $messageId)->update(...)`。
//
// 🔴🔴 这里刻意按**主键 id** 更新，与查询用的 message_id 不是一回事 —— 这是原项目的 bug，
// 按"不改动原项目逻辑"的要求照搬。详见 biz 层 MessageDetailUsecase.Detail 的说明。
//
// ⚠️ 不要"顺手"改成 message_id。要改需业务确认（那会改变线上行为）。
func (r *messageDetailRepo) MarkReadByPrimaryKey(ctx context.Context, primaryKey int64) error {
	return r.db.WithContext(ctx).
		Table(r.prefix+"messages").
		Where("id = ?", primaryKey).
		Update("is_read", 1).Error
}
