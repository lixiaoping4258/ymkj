package biz

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrSaleNotFound 对应控制器里的 `return $this->fail('未找到记录信息')`。
var ErrSaleNotFound = errors.New("未找到记录信息")

// SaleDetailUsecase 对应 app/api/logic/market/SaleLogic.php::getSaleDetail。
type SaleDetailUsecase struct {
	repo SaleDetailRepo
	cfg  *ConfigUsecase
}

func NewSaleDetailUsecase(repo SaleDetailRepo, cfg *ConfigUsecase) *SaleDetailUsecase {
	return &SaleDetailUsecase{repo: repo, cfg: cfg}
}

// SaleDetailRepo 秒转详情。
type SaleDetailRepo interface {
	// FindSale 对应：
	//
	//	MarketSales::alias('market_sales')
	//	    ->where(['market_sales.id'=>$id, 'market_sales.state'=>['ON_SALE','SELL_OUT']])
	//	    ->join('app_archive','market_sales.archive_id=app_archive.id')
	//	    ->join('app','market_sales.app_id=app.id')
	//	    ->field([...12 列...])->findOrEmpty();
	FindSale(ctx context.Context, id int64) (map[string]any, bool, error)
	// FindSaler 对应 `User::where(['id'=>$uid])->field('nickname,avatar')->findOrEmpty()`
	FindSaler(ctx context.Context, userID int64) (nickname string, avatar string, found bool, err error)
	// CountSalesByState 对应 `MarketSales::where(['user_id'=>..,'state'=>[..]])->count()`
	CountSalesByState(ctx context.Context, userID int64, state string) (int64, error)
}

// SalesStateSellOut 对应 x_market_sales.state 的枚举值
// （enum('ON_SALE','SELL_OUT','OFFLINE')；SELL_OUT 当前 0 行但枚举里有）。
const SalesStateSellOut = "SELL_OUT"

// SaleDetailResult 对应 getSaleDetail 的返回：
// `['collection'=>..., 'saler'=>..., 'trade_notice'=>...]`。
type SaleDetailResult struct {
	// Collection 12 列，images 已 json_decode 成数组
	Collection map[string]any
	// Saler 查不到用户时是**空数组 []**（不是 null）
	Saler map[string]any
	// TradeNotice 来自 `ConfigService::get('trade','notice')` —— **无默认值，可能 null**
	TradeNotice any
}

// GetSaleDetail 逐字对应 SaleLogic::getSaleDetail($id)。
//
// 原文：
//
//	$saleInfo = MarketSales::alias('market_sales')
//	    ->where(['market_sales.id'=>$id, 'market_sales.state'=>['ON_SALE','SELL_OUT']])
//	    ->join('app_archive','market_sales.archive_id=app_archive.id')
//	    ->join('app','market_sales.app_id=app.id')
//	    ->field([...])->findOrEmpty();
//	if ($saleInfo->isEmpty()) { return false; }               // -> fail('未找到记录信息')
//	$saleInfo = $saleInfo->toArray();
//	if (!empty($saleInfo['images'])) { $saleInfo['images'] = json_decode($saleInfo['images'], true); }
//
//	$ret = ['collection'=>$saleInfo, 'saler'=>[]];
//	$salerInfo = User::where(['id'=>$saleInfo['user_id']])->field('nickname,avatar')->findOrEmpty();
//	if (!$salerInfo->isEmpty()) {
//	    $saler = $salerInfo->toArray();
//	    $ret['saler'] = $saler;
//	    $ret['saler']['sale_count']       = MarketSales::where(['user_id'=>..,'state'=>['ON_SALE']])->count();
//	    $ret['saler']['done_count']       = MarketSales::where(['user_id'=>..,'state'=>['SELL_OUT']])->count();
//	    $ret['saler']['collection_count'] = 0;               // ⚠️ 硬编码 0
//	}
//	$notice = WebSettingLogic::getTradeNotice();             // ['content'=>ConfigService::get('trade','notice')]
//	$ret['trade_notice'] = $notice['content'];
//	return $ret;
//
// ⚠️ `state` 用的是 **IN ('ON_SALE','SELL_OUT')** —— 下架(OFFLINE)的记录查不到。
// ⚠️ `collection_count` **恒为 0**（硬编码），不是查出来的。
// ⚠️ 用户查不到时 `saler` 是 **空数组 []**（而且不会补 sale_count 等键）。
// ⚠️ `avatar` 经 `User::getAvatarAttr` -> `FileService::getFileUrl` 拼域名
//
//	（空值变成空串）。域名由 handler 传入。
//
// ⚠️ `trade_notice` 是 `ConfigService::get('trade','notice')` —— **没有默认值**，
//
//	配置缺失时是 **null**。
func (uc *SaleDetailUsecase) GetSaleDetail(ctx context.Context, id int64, domain string) (*SaleDetailResult, error) {
	collection, found, err := uc.repo.FindSale(ctx, id)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrSaleNotFound
	}

	// if (!empty($saleInfo['images'])) { json_decode(...) }
	if s, ok := collection["images"].(string); ok && s != "" {
		var v any
		if json.Unmarshal([]byte(s), &v) == nil {
			collection["images"] = v
		}
	}

	res := &SaleDetailResult{
		Collection: collection,
		Saler:      map[string]any{}, // 默认空数组（PHP 的 []）
	}

	if uid, ok := toInt64Any(collection["user_id"]); ok {
		nickname, avatar, salerFound, err := uc.repo.FindSaler(ctx, uid)
		if err != nil {
			return nil, err
		}
		if salerFound {
			res.Saler = map[string]any{
				"nickname": nickname,
				// 对应 getAvatarAttr: trim($v) ? getFileUrl($v) : ''
				"avatar": joinFileURL(domain, avatar),
			}
			onSale, err := uc.repo.CountSalesByState(ctx, uid, SalesStateOnSale)
			if err != nil {
				return nil, err
			}
			done, err := uc.repo.CountSalesByState(ctx, uid, SalesStateSellOut)
			if err != nil {
				return nil, err
			}
			res.Saler["sale_count"] = onSale
			res.Saler["done_count"] = done
			// ⚠️ 原文硬编码 0，不是查出来的
			res.Saler["collection_count"] = 0
		}
	}

	// $notice = WebSettingLogic::getTradeNotice(); $ret['trade_notice'] = $notice['content'];
	notice, err := uc.cfg.GetDefault(ctx, "trade", "notice", nil)
	if err != nil {
		return nil, err
	}
	res.TradeNotice = notice
	return res, nil
}
