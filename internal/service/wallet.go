package service

import (
	"context"
	"fmt"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// WalletService 对应 app/api/controller/v1/wallet/UserWalletController.php。
type WalletService struct {
	v1.UnimplementedWalletServiceServer
	balance *biz.BalanceQueryUsecase
}

func NewWalletService(balance *biz.BalanceQueryUsecase) *WalletService {
	return &WalletService{balance: balance}
}

// GetBalance 对应 UserWalletController::balance（GET /v1/wallet/balance/query）。
//
// 原实现：
//
//	$params = (new ChannelBalanceValidate())->get()->goCheck();
//	$value = BalanceWalletLogic::queryBalance($this->userId, $params['channel']);
//	return $this->data(['balance' => $value]);
//
// ⚠️ `$value` 的类型是**变化的**（int 0 / int -1 / string 余额 / null），
//
//	所以这里手工拼 JSON，而不是走 proto 消息 —— 用 proto 会把所有分支
//	都收敛成同一种类型（例如 null 变成 ""、string 变成数字），与 PHP 不一致。
func (s *WalletService) GetBalance(
	ctx context.Context, req *v1.GetBalanceRequest,
) (*v1.RawData, error) {
	// 校验文案对应 ChannelBalanceValidate 的 'channel.integer' => '兑换渠道不正确'
	if req.Channel == nil {
		return nil, bizFail(biz.NewBizError("兑换渠道不正确"))
	}
	userID := biz.UserIDFromContext(ctx)
	value, err := s.balance.QueryBalance(ctx, uint32(userID), req.Channel.Value)
	if err != nil {
		return nil, err
	}

	// 按值类型分别序列化，保住 PHP 的原始类型
	switch v := value.(type) {
	case nil:
		return &v1.RawData{Json: []byte(`{"balance":null}`)}, nil
	case int:
		return &v1.RawData{Json: []byte(fmt.Sprintf(`{"balance":%d}`, v))}, nil
	case string:
		b, merr := biz.MarshalNoHTMLEscape(map[string]any{"balance": v})
		if merr != nil {
			return nil, merr
		}
		return &v1.RawData{Json: b}, nil
	default:
		b, merr := biz.MarshalNoHTMLEscape(map[string]any{"balance": v})
		if merr != nil {
			return nil, merr
		}
		return &v1.RawData{Json: b}, nil
	}
}
