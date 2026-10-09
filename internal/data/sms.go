package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// smsRepo 对应 app/common/model/notice/SmsLog.php 的查询。
type smsRepo struct {
	db     *gorm.DB
	prefix string
}

func NewSmsRepo(db *gorm.DB, c *conf.Data) biz.SmsRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	return &smsRepo{db: db, prefix: prefix}
}

// FindLatestUnverified 逐字对应：
//
//	$where = [
//	    ['mobile','=',$mobile],
//	    ['send_status','=',SmsEnum::SEND_SUCCESS],
//	    ['scene_id','in',NoticeEnum::SMS_SCENE],
//	    ['is_verify','=',YesNoEnum::NO],
//	];
//	if(!empty($sceneId)) { $where[] = ['scene_id','=',$sceneId]; }
//	SmsLog::where($where)->order('send_time','desc')->findOrEmpty();
//
// ⚠️ 注意 `scene_id in SMS_SCENE` 是**始终存在**的条件，sceneId 只是**额外**再限定一次。
//
//	漏掉前者会把非短信场景的记录也读进来。
//
// ⚠️ x_sms_log.delete_time 是 int(10)。原文这里**没有**加 delete_time 过滤
//
//	（既没有 whereRaw 也不是软删除模型），所以照搬**不加**。
func (r *smsRepo) FindLatestUnverified(
	ctx context.Context, mobile string, sceneID int32,
) (*biz.SmsLogRow, error) {
	var row struct {
		ID         int64  `gorm:"column:id"`
		Code       string `gorm:"column:code"`
		IsVerify   int32  `gorm:"column:is_verify"`
		CheckNum   int32  `gorm:"column:check_num"`
		SendTime   int64  `gorm:"column:send_time"`
		Mobile     string `gorm:"column:mobile"`
		SceneID    int32  `gorm:"column:scene_id"`
		SendStatus int32  `gorm:"column:send_status"`
	}
	q := r.db.WithContext(ctx).
		Table(r.prefix+"sms_log").
		Select("id", "code", "is_verify", "check_num", "send_time", "mobile", "scene_id", "send_status").
		Where("mobile = ?", mobile).
		Where("send_status = ?", biz.SmsSendSuccess).
		Where("scene_id IN ?", biz.NoticeSmsScene).
		Where("is_verify = ?", biz.YesNoNo)

	// if(!empty($sceneId)) { $where[] = ['scene_id','=',$sceneId]; }
	//
	// ⚠️ PHP 的 empty(0) 为真 —— 所以 sceneId 传 0 时**不加**这个条件。
	if sceneID != 0 {
		q = q.Where("scene_id = ?", sceneID)
	}

	err := q.Order("send_time DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.SmsLogRow{
		ID: row.ID, Code: row.Code, IsVerify: row.IsVerify, CheckNum: row.CheckNum,
		SendTime: row.SendTime, Mobile: row.Mobile, SceneID: row.SceneID, SendStatus: row.SendStatus,
	}, nil
}

// SaveCheck 对应：
//
//	$smsLog->check_num = $smsLog->check_num + 1;
//	$smsLog->is_verify = YesNoEnum::YES;   // 仅成功时
//	$smsLog->save();
//
// ⚠️ 只更新这两列（不要整行 save，会把没读出来的列覆盖成零值）。
func (r *smsRepo) SaveCheck(ctx context.Context, id int64, checkNum int32, isVerify int32) error {
	return r.db.WithContext(ctx).
		Table(r.prefix+"sms_log").
		Where("id = ?", id).
		Updates(map[string]any{
			"check_num":   checkNum,
			"is_verify":   isVerify,
			"update_time": time.Now().Unix(),
		}).Error
}
