package data

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/data/model"
)

type userRepo struct {
	db *gorm.DB
}

// NewUserRepo 返回**具体类型**而不是接口。
//
// 因为同一个实现要同时满足 biz.UserProfileRepo 和 biz.UserBriefRepo 两个接口，
// 由 wire.Bind 去绑定（见 data.go 的 ProviderSet）。
// 如果这里直接返回接口类型，wire 就没法把它再绑到第二个接口上。
func NewUserRepo(db *gorm.DB) *userRepo {
	return &userRepo{db: db}
}

// notDeleted 对应 ThinkPHP SoftDelete 的过滤条件。
//
// User / UserAccounts 两个模型都用了 SoftDelete（delete_time 为 int 且可空），
// 所以查询必须带上这个条件，否则会把已注销用户也读出来。
const notDeleted = "delete_time IS NULL"

// FindProfile 对应：
//
//	User::where(['id'=>$userId])
//	    ->field('id,sn,sex,password,nickname,real_name,avatar,mobile,create_time,user_money,opt_pwd')
//	    ->findOrEmpty();
func (r *userRepo) FindProfile(ctx context.Context, userID uint64) (*biz.UserRow, error) {
	var row model.User
	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Select("id, sn, sex, password, nickname, real_name, avatar, mobile, create_time, user_money, opt_pwd").
		Where("id = ?", userID).
		Where(notDeleted).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.UserRow{
		ID:         row.ID,
		Sn:         row.Sn,
		Sex:        row.Sex,
		Nickname:   row.Nickname,
		RealName:   row.RealName,
		Avatar:     row.Avatar,
		Mobile:     row.Mobile,
		CreateTime: row.CreateTime,
		UserMoney:  row.UserMoney,
		Password:   row.Password,
		OptPwd:     row.OptPwd,
	}, nil
}

// IsRealVerified 对应：
//
//	UserReal::where(['user_id'=>$userId,'state'=>'SUCCESS'])->findOrEmpty()
//
// 只看记录是否存在，不关心内容。
func (r *userRepo) IsRealVerified(ctx context.Context, userID uint64) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.UserReal{}).
		Where("user_id = ? AND state = ?", userID, "SUCCESS").
		Limit(1).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// FindUserCode 对应：
//
//	UserAccounts::where(['user_id'=>$userId,'app_id'=>$appId])->value('user_code')
//
// 查不到时 ThinkPHP 的 value() 返回 null，上层 empty() 后取 ”。
// 这里统一返回 ""，由调用方判断。
func (r *userRepo) FindUserCode(ctx context.Context, userID uint64, appID int) (string, error) {
	var code *string
	err := r.db.WithContext(ctx).
		Model(&model.UserAccounts{}).
		Select("user_code").
		Where("user_id = ? AND app_id = ?", userID, appID).
		Where(notDeleted).
		Limit(1).
		Scan(&code).Error
	if err != nil {
		return "", err
	}
	if code == nil {
		return "", nil
	}
	return *code, nil
}

// FindBriefByID 取签发 token 缓存所需的基础字段。
//
// 同样要过滤软删除 —— 原项目这里靠 TP 的 SoftDelete 隐式完成。
func (r *userRepo) FindBriefByID(ctx context.Context, id uint64) (*biz.UserBrief, error) {
	var row model.User
	err := r.db.WithContext(ctx).
		Model(&model.User{}).
		Select("id, nickname, sn, mobile, avatar").
		Where("id = ?", id).
		Where(notDeleted).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.UserBrief{
		ID:       row.ID,
		Nickname: row.Nickname,
		Sn:       row.Sn,
		Mobile:   row.Mobile,
		Avatar:   row.Avatar,
	}, nil
}
