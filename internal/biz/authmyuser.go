package biz

import "context"

// AuthMyUserUsecase 对应 app/api/lists/open/AuthMyUserLists.php。
type AuthMyUserUsecase struct {
	repo AuthMyUserRepo
}

func NewAuthMyUserUsecase(repo AuthMyUserRepo) *AuthMyUserUsecase {
	return &AuthMyUserUsecase{repo: repo}
}

// AuthMyUserRepo 我授权的三方用户。
type AuthMyUserRepo interface {
	// List 对应 lists() —— **三张表**：
	//   AppAuth JOIN x_app App ON AppAuth.app_id=App.id
	//           JOIN x_user User ON AppAuth.uid=User.id
	List(ctx context.Context, userID uint32, offset, limit int) ([]AuthMyUserRow, error)
	// Count 对应 count() —— ⚠️ **只有两个 JOIN**（少 x_user），见 Usecase.List 的说明。
	Count(ctx context.Context, userID uint32) (int64, error)
}

// AuthMyUserRow 对应 field() 里的 7 列。
type AuthMyUserRow struct {
	// AppAuth.*
	ID          int64
	AppID       int32
	AppNickname string
	AppAvatar   string
	// ⚠️ expire 是 datetime；原实现**没有重新格式化**（与 message/detail 同类）
	Expire string
	// App.name as app_name
	AppName string
	// User.mobile —— 会被掩码（见 MaskMobile）
	Mobile string
}

// MaskMobile 逐字对应：
//
//	$item['mobile'] = !empty($item['mobile'])
//	    ? substr_replace($item['mobile'], '****', 3, 6)
//	    : '';
//
// ⚠️⚠️ `substr_replace($str, '****', 3, 6)` 的语义是：
//
//	**保留前 3 个字符 + 插入 '****' + 丢掉第 3~8 位（共 6 个）+ 保留第 9 位之后**
//
//	它**不是**"把中间 6 位换成 4 个星"那么简单 —— 星号只有 4 个而抹掉 6 个字符，
//	所以**结果字符串会比原串短 2 个字符**：
//
//	  13800138000 (11 位) -> 138****00   ( 9 位)   ← 短了 2 位！
//	  1380013800  (10 位) -> 138****0    ( 8 位)
//	  138001380   ( 9 位) -> 138****     ( 7 位)
//
//	如果按"中间打码"的直觉写成 `138****8000`，就与原项目不一致了。
//
// ⚠️ 长度不足时的 PHP 边界行为（offset 会被截断到字符串长度）：
//
//	'123' -> '123****'   （offset=3 正好等于长度 -> 直接追加）
//	'12'  -> '12****'    （offset 超出 -> 截断到 2）
//	''    -> ''          （外层 !empty 已拦掉）
func MaskMobile(mobile string) string {
	if mobile == "" {
		// 对应 `!empty($item['mobile'])` 的 else 分支：**空串**
		return ""
	}
	n := len(mobile) // 手机号是 ASCII，字节 == 字符
	if n <= 3 {
		// offset 被截断到 n -> 直接追加 '****'
		return mobile + "****"
	}
	head := mobile[:3]
	tail := ""
	if n > 9 {
		tail = mobile[9:]
	}
	return head + "****" + tail
}

// List 对应 `dataLists(new AuthMyUserLists())`。
//
// 🔴🔴 **原实现里 lists() 与 count() 的 JOIN 数量不一致，本实现刻意照搬**：
//
//	lists(): AppAuth JOIN x_app App JOIN x_user User      // 三张表
//	count(): AppAuth JOIN x_app App                       // ⚠️ 少 x_user
//
// 因为 `x_user` 那一条是 INNER JOIN（`AppAuth.uid = User.id`），
// 当某条授权的 uid 在 x_user 里不存在（用户被物理删除等）时：
//   - lists() 不会返回该行
//   - count() 却会把它算进去
//
// **于是会出现 count 大于实际列表长度的情况**（分页末页可能返回空列表）。
//
// 按"不改动原项目逻辑"照搬，**不把 count 补成三表 JOIN**。
//
// ⚠️ 掩码只在 lists() 里做，count() 不涉及。
func (uc *AuthMyUserUsecase) List(ctx context.Context, userID uint32, p PageParams) (*ListData, error) {
	count, err := uc.repo.Count(ctx, userID)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.List(ctx, userID, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}
	return &ListData{
		Lists:    rows,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}
