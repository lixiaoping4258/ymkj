package service

import (
	"context"

	v1 "github.com/lixiaoping4258/ymkj/api/xtravel/v1"
	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/pkg/pbconv"
)

// CommonService 对应原项目 app/api/controller/v1/common/*。
//
// Kratos 的分层：service 只做「协议转换」，业务判断全在 biz。
// 原项目 controller 里那种直接调 Service::get() 的写法在这里对应到 biz.ConfigUsecase。
type CommonService struct {
	v1.UnimplementedCommonServiceServer

	config *biz.ConfigUsecase
	trade  *biz.TradeConfigUsecase
}

func NewCommonService(config *biz.ConfigUsecase, trade *biz.TradeConfigUsecase) *CommonService {
	return &CommonService{config: config, trade: trade}
}

// GetConfig 对应 ConfigController::index。
//
// 原实现：
//
//	$config = [
//	    'outFee'          => ConfigService::get('trade','outFee'),
//	    'tradeSwitch'     => TradeConfigService::checkTradeTime(),
//	    'buyOrSaleConfig' => ConfigService::get('trade','buyOrSaleConfig'),
//	];
//	return $this->data($config);
//
// 注意原代码里 'tradeSwitch' 装的是 checkTradeTime() 的完整对象，
// 字段名和内容不符 —— 这是原项目的历史包袱，这里保持原样以兼容前端。
func (s *CommonService) GetConfig(ctx context.Context, _ *v1.GetConfigRequest) (*v1.GetConfigReply, error) {
	outFee, err := s.config.Get(ctx, "trade", "outFee")
	if err != nil {
		return nil, err
	}
	buyOrSale, err := s.config.Get(ctx, "trade", "buyOrSaleConfig")
	if err != nil {
		return nil, err
	}
	status, err := s.trade.CheckTradeTime(ctx)
	if err != nil {
		return nil, err
	}

	return &v1.GetConfigReply{
		OutFee:          pbconv.ToValue(outFee),
		TradeSwitch:     toTradeTimeStatus(status),
		BuyOrSaleConfig: pbconv.ToValue(buyOrSale),
	}, nil
}

// GetTradeConfig 对应 IndexController::tradeConfig -> IndexLogic::tradeConfig。
//
// 原实现有一层 20 秒的 JSON 缓存：
//
//	$key = 'trade:switch:limitTime';
//	if (cache($key)) { return json_decode(cache($key), true); }
//	$res['switch']    = ConfigService::get('trade','switch');
//	$res['limitTime'] = ConfigService::get('trade','limitTime');
//	cache($key, json_encode($res), 20);
//
// 这里**故意不加这层缓存**：ConfigService::get 自己已有 60 秒缓存，
// 再叠一层 20 秒的反而让配置改动要等两轮才生效，而且多一份缓存就多一处不一致。
// 如果实测 QPS 打不住再加，届时用同一套 biz.Cache 实现。
func (s *CommonService) GetTradeConfig(ctx context.Context, _ *v1.GetTradeConfigRequest) (*v1.GetTradeConfigReply, error) {
	sw, err := s.config.Get(ctx, "trade", "switch")
	if err != nil {
		return nil, err
	}
	limitTime, err := s.config.Get(ctx, "trade", "limitTime")
	if err != nil {
		return nil, err
	}
	return &v1.GetTradeConfigReply{
		Switch:    pbconv.ToValue(sw),
		LimitTime: pbconv.ToValue(limitTime),
	}, nil
}

func toTradeTimeStatus(st *biz.TradeTimeStatus) *v1.TradeTimeStatus {
	if st == nil {
		return nil
	}
	periods := make([]*v1.TradePeriod, 0, len(st.Periods))
	for _, p := range st.Periods {
		periods = append(periods, &v1.TradePeriod{Start: p.Start, End: p.End})
	}
	return &v1.TradeTimeStatus{
		TradeSwitch:  pbconv.ToValue(st.TradeSwitch),
		IsOpen:       st.IsOpen,
		TodayType:    st.TodayType,
		TodayTypeStr: st.TodayTypeStr,
		Periods:      periods,
		Tips:         st.Tips,
	}
}
