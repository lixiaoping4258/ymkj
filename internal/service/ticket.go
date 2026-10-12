package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// TicketService 对应 app/api/controller/v1/wallet/TicketWalletController.php。
type TicketService struct {
	v1.UnimplementedTicketServiceServer
	amount *biz.TicketAmountUsecase
	app    *conf.App
}

func NewTicketService(amount *biz.TicketAmountUsecase, app *conf.App) *TicketService {
	return &TicketService{amount: amount, app: app}
}

// GetTicketAmount 对应 TicketWalletController::amount（GET /v1/ticket/amount）。
//
// 原实现：
//
//	$value = AmountLogic::queryAmount($this->userId);
//	return $this->data($value);
//
// ⚠️ `limit_time` 的两个值来自 `.env` 的 APP.LIMITTIMESTART / LIMITTIMEEND，
//
//	Kratos 侧已迁到 `conf.App.limit_time_start / limit_time_end`
//	（见 conf.proto 的注释：**不能用 os.Getenv 兜底**，Go 服务读的是 YAML）。
func (s *TicketService) GetTicketAmount(
	ctx context.Context, _ *v1.GetTicketAmountRequest,
) (*v1.RawData, error) {
	userID := uint32(biz.UserIDFromContext(ctx))
	start, end := "", ""
	if s.app != nil {
		start = s.app.GetLimitTimeStart()
		end = s.app.GetLimitTimeEnd()
	}
	body, err := s.amount.Get(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}
	return &v1.RawData{Json: body}, nil
}
