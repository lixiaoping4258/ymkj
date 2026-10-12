package biz

import (
	"context"
	"errors"
	"strings"
)

// RealInfoUsecase 对应 app/api/logic/user/CardRealLogic.php。
type RealInfoUsecase struct {
	repo RealInfoRepo
}

func NewRealInfoUsecase(repo RealInfoRepo) *RealInfoUsecase {
	return &RealInfoUsecase{repo: repo}
}

// RealInfoRepo 实名信息。
type RealInfoRepo interface {
	// FindRealInfo 对应：
	//   UserReal::where(['user_id'=>$userId])
	//       ->field('state,card_no,user_name,birthday,gender,address')
	//       ->findOrEmpty()
	FindRealInfo(ctx context.Context, userID uint32) (*RealInfoRow, bool, error)
}

// RealInfoRow 对应 field('state,card_no,user_name,birthday,gender,address')。
type RealInfoRow struct {
	State    string
	CardNo   string
	UserName string
	Birthday *string
	Gender   *int32
	Address  *string
}

// 逐字对应原文的两条错误信息
var (
	ErrRealInfoNotFound = errors.New("未找到实名信息") // $real->isEmpty()
	ErrUserNotVerified  = errors.New("用户未实名")   // $real->state != 'SUCCESS'
)

// RealStateSuccess 对应 `$real->state != 'SUCCESS'` 里的字面量。
//
// x_user_real.state 是 enum('CREATE','SUCCESS','FAIL','PROCESS') —— 只有 SUCCESS 才放行。
const RealStateSuccess = "SUCCESS"

// RealInfoResult 是对外的实名信息（已脱敏）。
type RealInfoResult struct {
	State    string
	CardNo   string
	UserName string
	Birthday *string
	Gender   *int32
	Address  *string
}

// GetUserRealInfo 逐字对应 CardRealLogic::getUserRealInfo($userId)。
//
// 原文：
//
//	$real = UserReal::where(['user_id'=>$userId])
//	    ->field('state,card_no,user_name,birthday,gender,address')->findOrEmpty();
//	if ($real->isEmpty()) { self::setError('未找到实名信息'); return false; }
//	if ($real->state != 'SUCCESS') { self::setError('用户未实名'); return false; }
//	$info = $real->toArray();
//	$info['card_no']   = StrUtil::hideStr($info['card_no']);
//	$info['user_name'] = StrUtil::hideName($info['user_name']);
//	return $info;
//
// ⚠️ 两次校验的**顺序**：先"找不到"（未找到实名信息），再"状态不对"（用户未实名）。
//
//	反了会给未实名用户返回"未找到实名信息"。
func (uc *RealInfoUsecase) GetUserRealInfo(ctx context.Context, userID uint32) (*RealInfoResult, error) {
	row, found, err := uc.repo.FindRealInfo(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found || row == nil {
		return nil, ErrRealInfoNotFound
	}
	if row.State != RealStateSuccess {
		return nil, ErrUserNotVerified
	}
	return &RealInfoResult{
		State:    row.State,
		CardNo:   HideStr(row.CardNo),
		UserName: HideName(row.UserName),
		Birthday: row.Birthday,
		Gender:   row.Gender,
		Address:  row.Address,
	}, nil
}

// HideStr 逐字对应 xlu\StrUtil::hideStr。
//
//	$length = mb_strlen($str);
//	if ($length <= 2) { return $str; }        // ⚠️ 长度 ≤2 **原样返回**
//	$first = mb_substr($str, 0, 1);
//	$last  = mb_substr($str, -1, 1);
//	return $first . str_repeat('*', $length - 2) . $last;
//
// ⚠️⚠️ **必须按字符（rune）算，不能按字节**。PHP 用的是 `mb_*` 系列，
//
//	对中文/多字节是"字符"语义。Go 里 len() 是字节数、[]rune 才是字符数。
//	身份证号是 ASCII 所以两者相同，但同一个函数也会用在别处 —— 统一按 rune 写。
func HideStr(s string) string {
	r := []rune(s)
	n := len(r)
	if n <= 2 {
		return s // ⚠️ 长度 ≤ 2 不脱敏，原文如此
	}
	return string(r[0]) + strings.Repeat("*", n-2) + string(r[n-1])
}

// HideName 逐字对应 xlu\StrUtil::hideName。
//
//	if (empty($name)) return '';
//	$len = mb_strlen($name, 'utf-8');
//	return mb_substr($name, 0, 1, 'utf-8') . str_repeat('*', $len - 1);
//
// 首字符 + (字符数-1) 个星。例：`郎云霞` -> `郎**`；`李` -> `李`。
//
// ⚠️ 同样是**字符**语义（mb_*）。用字节会得到乱码。
func HideName(name string) string {
	if name == "" {
		return ""
	}
	r := []rune(name)
	return string(r[0]) + strings.Repeat("*", len(r)-1)
}
