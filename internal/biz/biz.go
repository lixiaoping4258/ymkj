package biz

import (
	"github.com/google/wire"
)

// ProviderSet 是 biz 层的依赖注入集合。
// 对应原项目里那些到处 static 调用的 Service / Logic 类。
var ProviderSet = wire.NewSet(
	NewConfigUsecase,
	NewTradeConfigUsecase,
	NewUserTokenUsecase,
	NewUserUsecase,
	NewMarketUsecase,
	NewWhitelistUsecase,
	NewPurchaseFaceUsecase,
	NewSaleFaceUsecase,
	NewStockUsecase,
	NewArchiveUsecase,
	NewFeeUsecase,
	NewPurchaseStateUsecase,
	NewStaleCache,
)
