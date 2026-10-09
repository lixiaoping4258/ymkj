package biz

import (
	"context"
	"time"

	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// 本文件对应原项目 app/common/service/sms/SmsDriver.php 的 verify()。
//
// ⚠️ 只迁移**校验**部分。发送部分（send / sendLimit / 各短信引擎）需要真实的
// 短信服务商配置与 SDK，未迁。

// 逐字对应 app/common/enum/notice/NoticeEnum.php
const (
	// NoticeLoginCaptcha = NoticeEnum::LOGIN_CAPTCHA —— 登录验证码的场景号
	NoticeLoginCaptcha = 101
	// NoticeBindMobileCaptcha / ChangeMobile / FindLoginPassword / ResetOptPassword / Unregister
	NoticeBindMobileCaptcha   = 102
	NoticeChangeMobileCaptcha = 103
	NoticeFindLoginPwdCaptcha = 104
	NoticeResetOptPassword    = 105
	NoticeUnregister          = 106
)

// NoticeSmsScene 逐字对应 NoticeEnum::SMS_SCENE。
//
//	const SMS_SCENE = [
//	    self::LOGIN_CAPTCHA,             // 101
//	    self::BIND_MOBILE_CAPTCHA,       // 102
//	    self::CHANGE_MOBILE_CAPTCHA,     // 103
//	    self::FIND_LOGIN_PASSWORD_CAPTCHA, // 104
//	    self::RESET_OPT_PASSWORD,        // 105
//	    self::UNREGISTER,                // 106
//	];
var NoticeSmsScene = []int32{
	NoticeLoginCaptcha, NoticeBindMobileCaptcha, NoticeChangeMobileCaptcha,
	NoticeFindLoginPwdCaptcha, NoticeResetOptPassword, NoticeUnregister,
}

// 逐字对应 app/common/enum/notice/SmsEnum.php
const (
	SmsSendIng     = 0
	SmsSendSuccess = 1
	SmsSendFail    = 2
)

// 逐字对应 app/common/enum/YesNoEnum.php
const (
	YesNoNo  = 0
	YesNoYes = 1
)

// SmsLogRow 是校验用到的 x_sms_log 列。
type SmsLogRow struct {
	ID         int64
	Code       string
	IsVerify   int32
	CheckNum   int32
	SendTime   int64
	Mobile     string
	SceneID    int32
	SendStatus int32
}

// SmsRepo 短信日志端口。
type SmsRepo interface {
	// FindLatestUnverified 对应 verify() 里的查询：
	//
	//	$where = [
	//	    ['mobile','=',$mobile],
	//	    ['send_status','=',SmsEnum::SEND_SUCCESS],
	//	    ['scene_id','in',NoticeEnum::SMS_SCENE],
	//	    ['is_verify','=',YesNoEnum::NO],
	//	];
	//	if(!empty($sceneId)) { $where[] = ['scene_id','=',$sceneId]; }
	//	$smsLog = SmsLog::where($where)->order('send_time','desc')->findOrEmpty();
	FindLatestUnverified(ctx context.Context, mobile string, sceneID int32) (*SmsLogRow, error)
	// SaveCheck 对应 `$smsLog->check_num = ...; $smsLog->is_verify = ...; $smsLog->save();`
	SaveCheck(ctx context.Context, id int64, checkNum int32, isVerify int32) error
}

// SmsUsecase 短信验证码校验。
type SmsUsecase struct {
	repo SmsRepo
	// appDebug 对应 env('app.debug')。
	//
	// ⚠️⚠️ **它开启一个万能验证码后门**：
	//
	//	if(env('app.debug')) {
	//	    if($code == '7700') { return true; }
	//	}
	//
	// 只要 app.debug 为真，**任何手机号 + 验证码 7700 都能通过校验**。
	// 本项目 .env 里 APP_DEBUG = true，所以这个后门是**生效的**。
	// 按"不改动原项目逻辑"照搬；但已记入 README 的安全章节 ——
	// **生产环境必须把 APP_DEBUG 设为 false**，否则等于没有短信校验。
	appDebug bool
}

func NewSmsUsecase(repo SmsRepo, app *conf.App) *SmsUsecase {
	debug := false
	if app != nil {
		debug = app.Debug
	}
	return &SmsUsecase{repo: repo, appDebug: debug}
}

// SmsVerifyTTL 对应 `$smsLog->send_time < time() - 5 * 60` —— 有效期 5 分钟。
const SmsVerifyTTLSeconds = 5 * 60

// SmsMaxCheckNum 对应 `if($smsLog->check_num > 5) { return false; }`
const SmsMaxCheckNum = 5

// SmsDebugBypassCode 对应后门里的 `'7700'`。
const SmsDebugBypassCode = "7700"

// Verify 逐字对应 SmsDriver::verify($mobile, $code, $sceneId)。
//
// 原文：
//
//	if(env('app.debug')) { if($code == '7700') { return true; } }
//	$smsLog = SmsLog::where([...])->order('send_time','desc')->findOrEmpty();
//	if($smsLog->isEmpty()) { return false; }
//	if($smsLog->check_num > 5) { return false; }
//	if($smsLog->is_verify || ($smsLog->send_time < time() - 5*60)) { return false; }
//	if($smsLog->code == $code) {
//	    $smsLog->check_num += 1; $smsLog->is_verify = YES; $smsLog->save(); return true;
//	}
//	$smsLog->check_num += 1; $smsLog->save(); return false;
//
// ⚠️ 注意判定顺序：**先查记录，再判 check_num，再判 is_verify/过期，最后比对**。
//
//	而且 `check_num` 是在**比对之后**自增的（成功和失败都会自增）。
func (uc *SmsUsecase) Verify(ctx context.Context, mobile, code string, sceneID int32) (bool, error) {
	// if(env('app.debug')) { if($code == '7700') { return true; } }
	if uc.appDebug && code == SmsDebugBypassCode {
		return true, nil
	}

	row, err := uc.repo.FindLatestUnverified(ctx, mobile, sceneID)
	if err != nil {
		return false, err
	}
	// if($smsLog->isEmpty()) { return false; }
	if row == nil {
		return false, nil
	}
	// if($smsLog->check_num > 5) { return false; }
	if row.CheckNum > SmsMaxCheckNum {
		return false, nil
	}
	// if($smsLog->is_verify || ($smsLog->send_time < time() - 5*60)) { return false; }
	if row.IsVerify == YesNoYes || row.SendTime < time.Now().Unix()-SmsVerifyTTLSeconds {
		return false, nil
	}

	// if($smsLog->code == $code) { ... return true; }
	if row.Code == code {
		if err := uc.repo.SaveCheck(ctx, row.ID, row.CheckNum+1, YesNoYes); err != nil {
			return false, err
		}
		return true, nil
	}
	// 失败也自增 check_num
	if err := uc.repo.SaveCheck(ctx, row.ID, row.CheckNum+1, row.IsVerify); err != nil {
		return false, err
	}
	return false, nil
}
