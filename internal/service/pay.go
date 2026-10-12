package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// PayService 对应 app/api/controller/v1/payment/PaymentController.php。
type PayService struct {
	v1.UnimplementedPayServiceServer
	pay *biz.PayChannelUsecase
}

func NewPayService(pay *biz.PayChannelUsecase) *PayService {
	return &PayService{pay: pay}
}

// GetChannel 对应 PaymentController::channel（GET /v1/payment/channel）。
//
// 原实现：
//
//	public function channel(): Json {
//	    $data = PaymentLogic::getScenePayWay(PayEnum::SCENE_MNP);
//	    return $this->data($data);
//	}
//
// ⚠️ `icon` 需要拼域名 —— 对应 `PayWay::getIconAttr` 调用的 `FileService::getFileUrl`。
//
//	域名取请求的 domain（与 login 域的 avatar 拼接同一套逻辑）。
func (s *PayService) GetChannel(
	ctx context.Context, _ *v1.GetChannelRequest,
) (*v1.GetChannelReply, error) {
	rows, err := s.pay.GetScenePayWay(ctx, biz.PaySceneMnp)
	if err != nil {
		return nil, err
	}
	domain := domainFrom(ctx)
	items := make([]*v1.PayChannelItem, 0, len(rows))
	for _, x := range rows {
		items = append(items, &v1.PayChannelItem{
			Id:        x.ID,
			IsDefault: x.IsDefault,
			Name:      x.Name,
			// 对应 getIconAttr -> FileService::getFileUrl：相对路径拼上域名
			Icon:     joinFileURLByDomain(domain, x.Icon),
			Remark:   x.Remark,
			NeedPass: x.NeedPass,
		})
	}
	return &v1.GetChannelReply{Items: items}, nil
}
