package data

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// loginRepo 对应 app\api\logic\LoginLogic.php + app\common\model\user\*。
//
// ⚠️ 两个必须记住的坑：
//
//  1. **x_user_accounts 有软删除**（`delete_time` datetime NULLABLE）。
//     实测建表结构确认。ThinkPHP 的 SoftDelete 会隐式加过滤，GORM 不会 ——
//     漏了会把已删除的账号查出来当成有效账号（同 MarketPurchase 那个 bug 一类）。
//
//  2. **x_user 也有软删除**，原实现靠 TP 隐式过滤。
type loginRepo struct {
	db     *gorm.DB
	prefix string
	ids    biz.IDGenerator
	// passwordSalt 对应 Config::get('project.unique_identification')
	passwordSalt string
	// defaultAvatar 对应 ConfigService::get('default_image','user_avatar')
	//
	// ⚠️ 注意与 login() 里用的 `project.default_image.user_avatar` **是不同的配置键**，
	// 两个都要读，不要合并。
	defaultAvatar string
}

// NewLoginRepo 装配登录仓储。
func NewLoginRepo(db *gorm.DB, c *conf.Data, ids biz.IDGenerator, auth *conf.Auth) biz.LoginRepo {
	prefix := "la_"
	if c != nil && c.Database != nil && c.Database.Prefix != "" {
		prefix = c.Database.Prefix
	}
	salt := ""
	if auth != nil {
		salt = auth.UniqueIdentification
	}
	return &loginRepo{db: db, prefix: prefix, ids: ids, passwordSalt: salt}
}

// 软删除过滤条件（两个表都有 delete_time）
const (
	notDeletedUserAccounts = "delete_time IS NULL"
	notDeletedUser         = "delete_time IS NULL"
)

// FindAccount 对应：
//
//	UserAccounts::where(['app_id'=>$appID,'type'=>$type,'account'=>$account])->findOrEmpty()
//
// ⚠️ 必须带 delete_time IS NULL（见文件头说明）。
func (r *loginRepo) FindAccount(
	ctx context.Context, appID, accountType int, account string,
) (*biz.UserAccountRow, bool, error) {
	var row struct {
		ID     uint64 `gorm:"column:id"`
		UserID uint64 `gorm:"column:user_id"`
		State  string `gorm:"column:state"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user_accounts").
		Select("id, user_id, state").
		Where("app_id = ? AND type = ? AND account = ?", appID, accountType, account).
		Where(notDeletedUserAccounts).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &biz.UserAccountRow{ID: row.ID, UserID: row.UserID, State: row.State}, true, nil
}

// FindUserByID 对应 User::where(['id'=>$userId])->findOrEmpty()（带软删除过滤）。
func (r *loginRepo) FindUserByID(ctx context.Context, id uint64) (*biz.UserLoginRow, error) {
	var row struct {
		ID        uint64 `gorm:"column:id"`
		Nickname  string `gorm:"column:nickname"`
		Sn        uint64 `gorm:"column:sn"`
		Mobile    string `gorm:"column:mobile"`
		Avatar    string `gorm:"column:avatar"`
		IsDisable int32  `gorm:"column:is_disable"`
	}
	err := r.db.WithContext(ctx).
		Table(r.prefix+"user").
		Select("id, nickname, sn, mobile, avatar, is_disable").
		Where("id = ?", id).
		Where(notDeletedUser).
		Take(&row).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &biz.UserLoginRow{
		ID: row.ID, Nickname: row.Nickname, Sn: row.Sn,
		Mobile: row.Mobile, Avatar: row.Avatar, IsDisable: row.IsDisable,
	}, nil
}

// UpdateLoginInfo 对应：
//
//	$user->login_time = time(); $user->login_ip = request()->ip(); $user->save();
//
// ⚠️ 只更新这两列，不要用 Save()（整行保存会把没读出来的列覆盖成零值 ——
// 原代码在 expireToken 处踩过同类坑，见 user_session 的注释）。
func (r *loginRepo) UpdateLoginInfo(ctx context.Context, userID uint64, loginTime int64, loginIP string) error {
	return r.db.WithContext(ctx).
		Table(r.prefix+"user").
		Where("id = ?", userID).
		Updates(map[string]any{"login_time": loginTime, "login_ip": loginIP}).
		Error
}

// Register 对应 LoginLogic::register —— **两张表写入，必须同一事务**。
//
// 原实现（已逐行核对）：
//
//	Db::startTrans();
//	$userSn   = User::createUserSn();          // = Id::gen()
//	$password = create_password($params['password'], $salt);
//	$avatar   = ConfigService::get('default_image','user_avatar');
//	User::create(['sn'=>$userSn,'avatar'=>$avatar,'nickname'=>'用户'.$userSn,
//	              'password'=>$password,'channel'=>...,'mobile'=>$mobile]);
//	UserAccounts::create(['id'=>Id::gen(),'user_id'=>...,'account'=>...,
//	                      'app_id'=>$app_id,'type'=>$accountType,'create_time'=>now()]);
//	Db::commit();
//	return $account;                            // ← 返回 UserAccounts，不是 User
func (r *loginRepo) Register(ctx context.Context, p biz.RegisterParams) (*biz.UserAccountRow, error) {
	// createUserSn() 现在就是 Id::gen()
	sn, err := r.ids.Gen(ctx, 0)
	if err != nil {
		return nil, err
	}
	accountID, err := r.ids.Gen(ctx, 0)
	if err != nil {
		return nil, err
	}

	nickname := "用户" + itoa64(sn)
	avatar := r.defaultAvatar
	appID := p.AppID
	// 第三方才用 extData 覆盖（accountType == 3）
	if p.AccountType == biz.AccountTypeThird && p.ExtData != nil {
		if v, ok := p.ExtData["avatar"].(string); ok && v != "" {
			avatar = v
		}
		if v, ok := p.ExtData["nickname"].(string); ok && v != "" {
			nickname = v
		}
		if v, ok := p.ExtData["app_id"]; ok {
			appID = toIntFromAny(v, 1)
		}
	}
	if appID == 0 {
		appID = 1 // 原实现：非第三方时 app_id = 1
	}

	password := biz.CreatePassword(p.PlainPassword, r.passwordSalt)
	now := time.Now()

	var userID uint64
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) 建用户
		if err := tx.Table(r.prefix + "user").Create(map[string]any{
			"sn":          sn,
			"avatar":      avatar,
			"nickname":    nickname,
			"password":    password,
			"channel":     p.Channel,
			"mobile":      p.Mobile,
			"create_time": now,
			"update_time": now,
		}).Error; err != nil {
			return err
		}
		// 取回自增 id（原实现用 $user->id）
		var created struct {
			ID uint64 `gorm:"column:id"`
		}
		if err := tx.Table(r.prefix+"user").Select("id").
			Where("sn = ?", sn).Order("id DESC").Take(&created).Error; err != nil {
			return err
		}
		userID = created.ID

		// 2) 建绑定账号
		return tx.Table(r.prefix + "user_accounts").Create(map[string]any{
			"id":          accountID,
			"user_id":     userID,
			"account":     p.Account,
			"app_id":      appID,
			"type":        p.AccountType,
			"state":       "ENABLE",
			"create_time": now,
			"user_code":   "",
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &biz.UserAccountRow{ID: uint64(accountID), UserID: userID, State: "ENABLE"}, nil
}

// CreateAppNotify 对应 AppNotifyListener → OpenAppsNotifyLogic::create。
//
// ⚠️ **尚未实现**：`OpenAppsNotifyLogic::create` 的字段映射与目标表
// `x_open_apps_notify` 的列都没有核对过，所以这里**明确返回错误**而不是静默跳过 ——
// 静默跳过会让三方通知悄悄丢失。
//
// 影响面：只有 `app_id == 2`（第三方登录）会走到；
// **account 密码登录的 app_id 恒为 1，不受影响**。
func (r *loginRepo) CreateAppNotify(
	_ context.Context, action, appID string, userID uint64, body map[string]any,
) error {
	return errors.New("CreateAppNotify 未实现：需先核对 OpenAppsNotifyLogic::create 的字段与 x_open_apps_notify 表结构")
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [24]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func toIntFromAny(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n := 0
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				return def
			}
			n = n*10 + int(t[i]-'0')
		}
		return n
	default:
		return def
	}
}
