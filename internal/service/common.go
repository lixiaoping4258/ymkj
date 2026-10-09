package service

import (
	"context"
	"github.com/go-kratos/kratos/v2/log"

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

	config   *biz.ConfigUsecase
	trade    *biz.TradeConfigUsecase
	platform *biz.PlatformUsecase
	log      *log.Helper
}

func NewCommonService(
	config *biz.ConfigUsecase, trade *biz.TradeConfigUsecase,
	platform *biz.PlatformUsecase, logger log.Logger,
) *CommonService {
	return &CommonService{config: config, trade: trade, platform: platform, log: log.NewHelper(logger)}
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

// GetProtocol 对应 IndexController::policy（GET /v1/common/protocol）。
//
// 原实现：
//
//	$type = $this->request->get('type/s', '');
//	$result = IndexLogic::getPolicyByType($type);
//	return $this->data($result);
//
// 而 getPolicyByType 是：
//
//	return [
//	    'title'   => ConfigService::get('agreement', $type.'_title', ''),
//	    'content' => ConfigService::get('agreement', $type.'_content', ''),
//	];
//
// ⚠️ 配置缺失时两个键都是**空字符串**（默认值 ”），不是 null ——
//
//	所以用 GetDefault(..., "") 而不是 Get(...)。
func (s *CommonService) GetProtocol(
	ctx context.Context, req *v1.GetProtocolRequest,
) (*v1.GetProtocolReply, error) {
	typ := req.GetType()

	title, err := s.config.GetDefault(ctx, "agreement", typ+"_title", "")
	if err != nil {
		return nil, err
	}
	content, err := s.config.GetDefault(ctx, "agreement", typ+"_content", "")
	if err != nil {
		return nil, err
	}
	return &v1.GetProtocolReply{
		Title:   toStr(title),
		Content: toStr(content),
	}, nil
}

// GetPlatformLists 对应 PlatformController::index（GET /v1/common/platform/lists）。
//
// 原实现：
//
//	$data = OpenAppLogic::getThirdAppsList();
//	return $this->data($data);
func (s *CommonService) GetPlatformLists(
	ctx context.Context, _ *v1.GetPlatformListsRequest,
) (*v1.GetPlatformListsReply, error) {
	items, err := s.platform.ListThirdApps(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.PlatformItem, 0, len(items))
	for _, x := range items {
		out = append(out, &v1.PlatformItem{Name: x.Name, Flag: x.Flag})
	}
	return &v1.GetPlatformListsReply{Items: out}, nil
}

// toStr 把配置值转成字符串。
//
// ⚠️ 配置值可能是 JSON 解出来的任意类型（ConfigService::get 会 json_decode）。
// 原实现直接把它塞进数组，json_encode 时按实际类型输出。
// 这里统一转字符串是因为 proto 字段是 string —— 若某个 agreement 配置存的是
// 数字或数组，输出形态会与 PHP 不同。当前 agreement 类型在库里**没有任何行**，
// 所以实际走不到；等真正配上内容时要复核这一点。
func toStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
