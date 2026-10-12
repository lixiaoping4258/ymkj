package biz

import "context"

// AuthUserUsecase 对应 app/api/lists/open/AppUserLists.php。
type AuthUserUsecase struct {
	repo AuthUserRepo
}

func NewAuthUserUsecase(repo AuthUserRepo) *AuthUserUsecase {
	return &AuthUserUsecase{repo: repo}
}

// AuthUserRepo 一级平台授权用户。
type AuthUserRepo interface {
	// List 对应 lists() —— `x_app_auth app_auth INNER JOIN x_user u`
	List(ctx context.Context, userID uint32, appID int64, offset, limit int) ([]AuthUserRow, error)
	// Count 对应 count() —— 🔴 **没有任何 JOIN**
	Count(ctx context.Context, userID uint32, appID int64) (int64, error)
	// ListAppInfo 对应 `(new AppLists())->listsWithQuery([['id','in',...]])`
	//
	// ⚠️ 条件是 `state=1 AND type=2 AND id IN (...)`，**7 列且不处理 logo**。
	// ⚠️ 它用的是**当前请求的**分页参数（offset/limit）—— 见 AuthUserUsecase.List 的说明。
	ListAppInfo(ctx context.Context, appIDs []int64, offset, limit int) ([]AppInfoRow, error)
}

// AuthUserRow 对应 lists() 的 7 个字段。
type AuthUserRow struct {
	ID          int64
	AppID       int32
	AppNickname string
	AppAvatar   string
	// start / expire 都是 datetime
	Start  string
	Expire string
	// Mobile 会在 Usecase.List 里被掩码（**length=4**，与 authMyUserList 的 6 不同）
	Mobile string
}

// AppInfoRow 对应 `AppLists::listsWithQuery` 的 field（**7 列**）：
// `id, name, state, logo, type, fee_rate, flag`
type AppInfoRow struct {
	ID      int64
	Name    string
	State   int32
	Logo    string
	Type    int32
	FeeRate string
	Flag    string
}

// AuthUserItem 是 ListData 里的一行（7 字段 + 嵌套 app_info）。
type AuthUserItem struct {
	AuthUserRow
	// AppInfo 可能为 nil —— 对应 PHP 的 `$appMap[$list['app_id']]` 取不到时
	// 产生 Undefined array key 并把值赋成 **null**。
	AppInfo *AppInfoRow
}

// MaskMobileLen4 对应 `substr_replace($mobile, '****', 3, 4)`。
//
// ⚠️⚠️ **与 authMyUserList 的掩码不同！** 那个是 length=6（结果比原串短 2 位），
//
//	这个是 length=4（结果**与原串等长**）。
//
// 期望值来自真实 PHP 实测：
//
//	'18300080008' -> '183****0008'  (11 -> 11)   ← 等长
//	'13800138000' -> '138****8000'  (11 -> 11)
//	'1234567890'  -> '123****890'   (10 -> 10)
//	'123'         -> '123****'      ( 3 ->  7)
//
// 语义：`substr($s,0,3) . '****' . substr($s,7)`，offset 超出时截断到长度。
// **不要复用 MaskMobile**（那个是 length=6）。
func MaskMobileLen4(mobile string) string {
	if mobile == "" {
		return ""
	}
	n := len(mobile)
	if n <= 3 {
		return mobile + "****"
	}
	tail := ""
	if n > 7 {
		tail = mobile[7:]
	}
	return mobile[:3] + "****" + tail
}

// List 对应 `dataLists(new AppUserLists())`。
//
// 原文 lists()：
//
//	$lists = AppAuth::alias("app_auth")->where($this->queryWhere())
//	    ->field('app_auth.id,app_auth.app_id,app_auth.app_nickname,app_auth.app_avatar,
//	             app_auth.start,app_auth.expire,u.mobile')
//	    ->join('user u', 'app_auth.uid = u.id')
//	    ->order('id', 'desc')
//	    ->limit(...)->select()->toArray();
//
//	$app_ids = array_column($lists, 'app_id');
//	$appLists = (new AppLists())->listsWithQuery([['id', 'in', implode(',', $app_ids)]]);
//	$appMap = MapHelper::mapWithKey($appLists, 'id');
//	foreach ($lists as &$list) {
//	    $list['app_info'] = $appMap[$list['app_id']];
//	    $list['mobile'] = substr_replace($list['mobile'], '****', 3, 4);
//	}
//
// ⚠️ `queryWhere()` = `uid = 当前用户 AND app_id = 入参`。
//
//	**"授权未过期"那行被注释掉了**：
//	    // $where[] = ['expire', '>', TimeHelper::now()];
//	-> **过期的授权也会返回**。照搬，不恢复。
//
// ⚠️ `order('id','desc')` 是**无限定名**的 id。已用真实 SQL 验证 MySQL 不报 ambiguous，
//
//	解析为左表 `app_auth.id`（实测结果与 app_auth.id DESC 一致）。
//
// ⚠️🔴 **`app_info` 有个隐藏截断**：`listsWithQuery` 内部用的是**当前请求的**
//
//	limit（`$this->limitOffset/limitLength`），所以它最多只返回 page_size 条 app。
//	同一页里 app_id 种类数超过 page_size 时，部分行的 `app_info` 会是 null。
//	照搬（传同一个 offset/limit）。
//
// ⚠️ `app_info` 取不到时 PHP 会产生 Undefined array key，值成为 **null** ——
//
//	不是 `{}`，也不是省略该键。
func (uc *AuthUserUsecase) List(
	ctx context.Context, userID uint32, appID int64, p PageParams,
) (*ListData, error) {
	count, err := uc.repo.Count(ctx, userID, appID)
	if err != nil {
		return nil, err
	}
	rows, err := uc.repo.List(ctx, userID, appID, p.Offset, p.Limit)
	if err != nil {
		return nil, err
	}

	// 收集本页出现过的 app_id（去重），对应 array_column + implode
	seen := map[int32]bool{}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if !seen[r.AppID] {
			seen[r.AppID] = true
			ids = append(ids, int64(r.AppID))
		}
	}

	appMap := map[int64]AppInfoRow{}
	if len(ids) > 0 {
		// ⚠️ 用**当前请求的**分页参数（原文 `$this->limitOffset/limitLength`）
		infos, err := uc.repo.ListAppInfo(ctx, ids, p.Offset, p.Limit)
		if err != nil {
			return nil, err
		}
		for _, a := range infos {
			appMap[a.ID] = a
		}
	}

	items := make([]AuthUserItem, 0, len(rows))
	for _, r := range rows {
		it := AuthUserItem{AuthUserRow: r, AppInfo: nil}
		it.Mobile = MaskMobileLen4(r.Mobile)
		if a, ok := appMap[int64(r.AppID)]; ok {
			cp := a
			it.AppInfo = &cp
		}
		items = append(items, it)
	}

	return &ListData{
		Lists:    items,
		Count:    count,
		PageNo:   p.PageNo,
		PageSize: p.PageSize,
		Extend:   []any{},
	}, nil
}
