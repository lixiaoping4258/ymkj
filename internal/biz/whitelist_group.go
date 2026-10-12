package biz

import (
	"context"
	"time"
)

// WhitelistGroupRepo 白名单组（group 接口用）。
//
// ⚠️ 与 WhitelistRepo 分开：那一个是中间件/权限判断用的（只看 enabled 标记），
// 这一个要返回**组的整行**给前端。
type WhitelistGroupRepo interface {
	// FindGroupRow 对应 `WhitelistGroup::find($whitelistUser['group_id'])`。
	//
	// ⚠️ 真实 SQL 是 `SELECT * FROM x_whitelist_group WHERE id = ?`（无软删除过滤，
	//    该表也没有 delete_time 列）。
	FindGroupRow(ctx context.Context, groupID uint64) (map[string]any, bool, error)
}

// WhitelistGroupResult 对应 `WhitelistLogic::getUserGroup($userId)` 的返回。
//
// 可能为 nil -> handler 输出 `{"group": null}`。
type WhitelistGroupResult struct {
	Group map[string]any
}

// GetUserGroup 逐字对应 WhitelistLogic::getUserGroup($userId)：
//
//	$whitelistUser = WhitelistUser::where('user_id', $userId)->find();
//	if (!$whitelistUser) { return null; }
//	$group = WhitelistGroup::find($whitelistUser['group_id']);
//	if (!$group || $group['status'] != 1) { return null; }
//	return $group->toArray();
//
// ⚠️⚠️ **status != 1 时返回 null**（不是返回组信息），所以"不在任何组"与
//
//	"在组里但组被禁用"两种情况**对外都是 null**，前端区分不了。
//
// ⚠️⚠️ `x_whitelist_group.create_time / update_time` 在库里是 **int(11) 的 unix 时间戳**，
//
//	但 ThinkPHP 模型的 autoWriteTimestamp 会在**读取时**把它们转成
//	`"Y-m-d H:i:s"` 字符串。**已用真实 PHP 对拍确认**：
//
//	  WhitelistGroup::find(68)->toArray()
//	  -> {"id":68,"name":"1","remark":"","status":1,
//	      "create_time":"2026-08-18 18:01:34","update_time":"2026-09-24 14:32:03"}
//
//	所以 Go 侧必须把 int 时间戳**格式化成字符串**，不能原样输出数字。
//	（对比：`x_user.create_time` 在 /v1/user/info 里输出的就是**数字** ——
//	  **每个模型不一样，必须逐个对拍**。）
func (uc *WhitelistUsecase) GetUserGroup(ctx context.Context, userID uint64) (*WhitelistGroupResult, error) {
	groupID, found, err := uc.repo.FindUserGroupID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !found {
		return &WhitelistGroupResult{Group: nil}, nil
	}
	row, ok, err := uc.groupRepo.FindGroupRow(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return &WhitelistGroupResult{Group: nil}, nil
	}
	// `$group['status'] != 1` -> null
	if st, ok2 := toInt64Any(row["status"]); !ok2 || st != 1 {
		return &WhitelistGroupResult{Group: nil}, nil
	}
	// int 时间戳 -> "Y-m-d H:i:s"（对齐 ThinkPHP 的 autoWriteTimestamp 读取转换）
	for _, k := range []string{"create_time", "update_time"} {
		if v, ok := row[k]; ok {
			if ts, ok2 := toInt64Any(v); ok2 {
				row[k] = FormatUnixAsDatetime(ts)
			}
		}
	}
	return &WhitelistGroupResult{Group: row}, nil
}

// FormatUnixAsDatetime 把 unix 秒格式化成 `Y-m-d H:i:s`。
//
// ⚠️ 用 `time.Local`（容器时区）。configs 里 TZ 是 Asia/Shanghai，
//
//	与 PHP 的 date() 一致 —— 但**部署时必须确保容器 TZ 也是它**，
//	否则同一时间戳会差 8 小时。
func FormatUnixAsDatetime(ts int64) string {
	return time.Unix(ts, 0).In(time.Local).Format("2006-01-02 15:04:05")
}
