package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// TicketAmountUsecase 对应 app/api/logic/ticket/AmountLogic.php::queryAmount。
type TicketAmountUsecase struct {
	repo  TicketAmountRepo
	cache RawCache // ⚠️ RedisLockService，**与 PHP 共用**
}

func NewTicketAmountUsecase(repo TicketAmountRepo, cache RawCache) *TicketAmountUsecase {
	return &TicketAmountUsecase{repo: repo, cache: cache}
}

// TicketAmountRepo 钱包汇总的三段查询。
type TicketAmountRepo interface {
	// ListWallet 对应：
	//   UserWallet::where(['user_id'=>$userId])
	//       ->field('amount,type_id,frozen_amount')->select()->toArray()
	//
	// ⚠️ 只取 3 列（表里共 7 列）；x_user_wallet **没有 delete_time**。
	ListWallet(ctx context.Context, userID uint32) ([]TicketWalletRow, error)
	// SumWaitAmount 对应：
	//   UserGetTicket::where(['user_id'=>..,'state'=>STATE_WAIT,'ticket_type'=>$typeID])
	//       ->sum('change_amount')
	//
	// ⚠️ 无匹配行时 ThinkPHP 返回 **0**（不是 null）。用 COALESCE(SUM(...),0) 对齐。
	// ⚠️ UserGetTicket extends Model、**无 use SoftDelete** -> 不加 delete_time。
	SumWaitAmount(ctx context.Context, userID uint32, typeID int32) (string, error)
	// WithdrawStatus 对应：
	//   UserAccounts::where(['user_id'=>..,'app_id'=>3,'state'=>'ENABLE'])->value('sync_order_num')
	//
	// 返回 (值, 是否取到)。⚠️ x_user_accounts **有软删除** -> 加 delete_time IS NULL。
	WithdrawStatus(ctx context.Context, userID uint32) (int64, bool, error)
}

// TicketWalletRow 对应 field('amount,type_id,frozen_amount')。
//
// ⚠️ amount / frozen_amount 是 decimal(10,2) -> PHP 输出**字符串**。
type TicketWalletRow struct {
	Amount       string
	TypeID       int32
	FrozenAmount string
}

// TicketAmountCacheKey 对应 `"user:queryAmount:$userId"`。
//
// ⚠️ 走 RedisLockService（裸 phpredis、plain JSON）-> **与 PHP 共用**（清单第 15 条）。
func TicketAmountCacheKey(userID uint32) string {
	return fmt.Sprintf("user:queryAmount:%d", userID)
}

// TicketAmountCacheTTL 对应 `RedisLockService::set($cacheKey, json_encode($return), 5)`。
const TicketAmountCacheTTL = 5 * time.Second

// 提票限制时间窗的两个 type_id —— **只有 2 和 4 会带 limit_time**。
const (
	ticketTypeWithdraw  int32 = 2 // 提票（带 limit_time + Withdraw_status）
	ticketTypeGetAmount int32 = 4 // 提票积分（带 limit_time）
)

// Get 逐字对应 AmountLogic::queryAmount($userId)。
//
// 原文：
//
//	$cacheKey = "user:queryAmount:$userId";
//	$ret = RedisLockService::get($cacheKey);
//	if ($ret) { return json_decode($ret, true); }
//
//	$envStart = env('APP.LIMITTIMESTART');   // -> limitTimeStart 参数
//	$envEnd   = env('APP.LIMITTIMEEND');     // -> limitTimeEnd 参数
//	$time = [$envStart, $envEnd];
//	$existType = [];
//	$res = UserWallet::where(['user_id'=>$userId])->field('amount,type_id,frozen_amount')->select()->toArray();
//	foreach ($res as $key => $value) {
//	    $existType[$value['type_id']] = true;
//	    switch ($value['type_id']) {
//	        case 4:
//	        case 2:
//	            $res[$key]['wait_amount'] = UserGetTicket::where([
//	                'user_id'=>$userId, 'state'=>CheckStateEnum::STATE_WAIT, 'ticket_type'=>$value['type_id'],
//	            ])->sum('change_amount');
//	            $res[$key]['limit_time'] = $time;
//	            break;
//	        case 1:
//	        default:
//	            $res[$key]['wait_amount'] = 0;          // ⚠️ 且**不加 limit_time**
//	            break;
//	    }
//	}
//	// 缺 1 / 2 / 3 / 4 各补一条零值（⚠️ **只有 2 和 4 带 limit_time**）
//	usort($res, fn($a,$b) => $a['type_id'] - $b['type_id']);   // 按 type_id 升序
//	foreach ($res as $key => $value) {
//	    if ($value['type_id'] == 2) {
//	        $sync_order_num = UserAccounts::where(['user_id'=>$userId,'app_id'=>3,'state'=>'ENABLE'])
//	            ->value('sync_order_num');
//	        $res[$key]['Withdraw_status'] = $sync_order_num > 0 ? 1 : 0;   // ⚠️ 大写 W
//	    }
//	}
//	$return = ['state'=>1, 'msg'=>'查询成功', 'list'=>$res];
//	RedisLockService::set($cacheKey, json_encode($return), 5);
//	return $return;
//
// ⚠️⚠️ `data` 内部**又套了一层 `{state, msg, list}`** ——
//
//	与外层信封的 code/show/msg/data 是**两层**，不要混淆也不要删。
//
// ⚠️ 补零行的 `amount` 是 PHP 字面量 **int 0**，而真实行是 **"0.00" 字符串** ——
//
//	同一批数据里类型不统一，照搬。
//
// ⚠️ 排序用 `usort` 且比较函数是 `$a['type_id'] - $b['type_id']` —— 按 type_id **升序**。
func (uc *TicketAmountUsecase) Get(
	ctx context.Context, userID uint32, limitTimeStart, limitTimeEnd string,
) (json.RawMessage, error) {
	cacheKey := TicketAmountCacheKey(userID)
	if raw, ok, err := uc.cache.Get(ctx, cacheKey); err == nil && ok && len(raw) > 0 {
		return raw, nil
	}

	timePair := []any{limitTimeStart, limitTimeEnd}

	rows, err := uc.repo.ListWallet(ctx, userID)
	if err != nil {
		return nil, err
	}

	existType := map[int32]bool{}
	list := make([]map[string]any, 0, len(rows)+4)
	for _, r := range rows {
		existType[r.TypeID] = true
		item := map[string]any{
			"amount":        r.Amount,
			"type_id":       r.TypeID,
			"frozen_amount": r.FrozenAmount,
		}
		switch r.TypeID {
		case 4, 2:
			wait, err := uc.repo.SumWaitAmount(ctx, userID, r.TypeID)
			if err != nil {
				return nil, err
			}
			item["wait_amount"] = wait
			item["limit_time"] = timePair
		default:
			// case 1 与 default 同分支：wait_amount 是 **int 0**，且没有 limit_time
			item["wait_amount"] = 0
		}
		list = append(list, item)
	}

	// 补齐缺失的 type_id（1/2/3/4）—— ⚠️ 只有 2 和 4 带 limit_time
	zeroEntry := func(typeID int32) map[string]any {
		m := map[string]any{
			"amount":        0, // ⚠️ int 0，与真实行的 "0.00" 字符串不同
			"type_id":       typeID,
			"frozen_amount": 0,
			"wait_amount":   0,
		}
		if typeID == 2 || typeID == 4 {
			m["limit_time"] = timePair
		}
		return m
	}
	for _, t := range []int32{1, 2, 3, 4} {
		if !existType[t] {
			list = append(list, zeroEntry(t))
		}
	}

	// 按 type_id 升序（usort）
	sort.SliceStable(list, func(i, j int) bool {
		return toInt32Any(list[i]["type_id"]) < toInt32Any(list[j]["type_id"])
	})

	// type_id == 2 时补 **大写 W** 的 Withdraw_status
	for _, item := range list {
		if toInt32Any(item["type_id"]) == ticketTypeWithdraw {
			n, found, err := uc.repo.WithdrawStatus(ctx, userID)
			if err != nil {
				return nil, err
			}
			// `$sync_order_num > 0` —— 取不到或为 NULL 时是 false -> 0
			if found && n > 0 {
				item["Withdraw_status"] = 1
			} else {
				item["Withdraw_status"] = 0
			}
		}
	}

	out := map[string]any{
		"state": 1,
		"msg":   "查询成功",
		"list":  list,
	}
	body, merr := MarshalNoHTMLEscape(out)
	if merr != nil {
		return nil, merr
	}
	_ = uc.cache.Set(ctx, cacheKey, body, TicketAmountCacheTTL)
	return body, nil
}

// toInt32Any map 里的 type_id（Go 侧自己放进去的，一定是 int32）
func toInt32Any(v any) int32 {
	switch x := v.(type) {
	case int32:
		return x
	case int64:
		return int32(x)
	case int:
		return int32(x)
	default:
		return 0
	}
}

// ErrTicketAmountNotFound 占位：本接口不会因为"查不到"而失败（缺数据时补零）。
var ErrTicketAmountNotFound = errors.New("查询成功")
