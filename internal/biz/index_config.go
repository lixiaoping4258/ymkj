package biz

import (
	"context"
	"encoding/json"
)

// IndexConfigUsecase 对应 IndexLogic::getConfigData()。
//
// 8 个顶层键，来源分散：4 段 x_config + 1 张表 + config/project.php 兜底 + 请求域名。
// 真机数据与逐项说明见 docs/RECON-index-config.md。
type IndexConfigUsecase struct {
	cfg  *ConfigUsecase
	repo IndexConfigRepo
}

func NewIndexConfigUsecase(cfg *ConfigUsecase, repo IndexConfigRepo) *IndexConfigUsecase {
	return &IndexConfigUsecase{cfg: cfg, repo: repo}
}

// IndexConfigRepo 底部导航表。
type IndexConfigRepo interface {
	// ListTabbar 对应 DecorateTabbar::getTabbarLists() 里的 `self::select()->toArray()`。
	//
	// ⚠️ **没有 where**（全表，不过滤 is_show）。
	// ⚠️ 表**没有 delete_time**，所以没有软删除过滤。
	ListTabbar(ctx context.Context) ([]TabbarRow, error)
}

// TabbarRow 对应 x_decorate_tabbar 一行。
//
// ⚠️ `link` 在模型里声明为 json 类型（`protected $json = ['link']` +
// `$jsonAssoc = true`），所以对外是**嵌套对象**而不是字符串。
type TabbarRow struct {
	ID         int64
	Name       string
	Selected   string
	Unselected string
	// LinkJSON 是列里的原始 JSON 字符串，对外要解析成对象
	LinkJSON   string
	Link       map[string]any
	IsShow     int32
	CreateTime int64
	UpdateTime int64
}

// TabbarStyle 对应 tabbar.style / project.decorate.tabbar_style。
type TabbarStyle struct {
	DefaultColor  string `json:"default_color"`
	SelectedColor string `json:"selected_color"`
}

// LoginConfig 对应 getConfigData 里的 $loginConfig（6 项）。
//
// ⚠️ 本项目 x_config **没有 login 段**，所以 6 个值全部来自 config/project.php：
//
//	'login_way'       => ['1', '2'],   // ⚠️ 字符串数组，不要转成数字
//	'coerce_mobile'   => 1,
//	'third_auth'      => 1,
//	'wechat_auth'     => 1,
//	'qq_auth'         => 0,
//	'login_agreement' => 1,
type LoginConfig struct {
	LoginWay       []string
	CoerceMobile   int32
	LoginAgreement int32
	ThirdAuth      int32
	WechatAuth     int32
	QqAuth         int32
}

// WebsiteConfig 对应 $website（3 项，其中 2 项经 getFileUrl）。
type WebsiteConfig struct {
	H5Favicon string
	ShopName  string
	ShopLogo  string
}

// WebPageConfig 对应 $webPage（4 项，url 用请求域名）。
type WebPageConfig struct {
	Status     int32
	PageStatus int32
	PageURL    string
	URL        string
}

// IndexConfigResult 对应 getConfigData 的返回值。
type IndexConfigResult struct {
	// Domain 对应 `FileService::getFileUrl()` —— ⚠️ **无参数**，
	// 等于 format(request()->domain(), '') = "<domain>/"（**带尾斜杠**）
	Domain  string
	Style   TabbarStyle
	Tabbar  []TabbarRow
	Login   LoginConfig
	Website WebsiteConfig
	WebPage WebPageConfig
	Version string
	// Copyright 对应 ConfigService::get('copyright','config',[])。
	// x_config 无该段 -> 空数组。PHP json_encode([]) = []（不是 {}）。
	Copyright []any
}

// project.php 里的固定值（实测）
const (
	ProjectVersion = "1.9.4"
	// 默认导航栏配色，与 x_config 的 tabbar.style 当前值相同
	DefaultTabbarStyleJSON = `{"default_color":"#999999","selected_color":"#c455ff"}`
)

// GetConfig 逐字对应 IndexLogic::getConfigData()。
//
// domain 与 webPage.url 都由 handler 传入（原文取 request()->domain()）。
func (uc *IndexConfigUsecase) GetConfig(ctx context.Context, requestDomain string) (*IndexConfigResult, error) {
	res := &IndexConfigResult{Version: ProjectVersion}

	// 'domain' => FileService::getFileUrl()
	// 无参数 -> format($domain, '') -> trim($domain) . '/' . '' -> 带尾斜杠
	res.Domain = trimRightSlash(requestDomain) + "/"

	// 'tabbar' => DecorateTabbar::getTabbarLists()
	tabbar, err := uc.repo.ListTabbar(ctx)
	if err != nil {
		return nil, err
	}
	res.Tabbar = tabbar

	// 'style' => ConfigService::get('tabbar','style', config('project.decorate.tabbar_style'))
	//
	// ⚠️ ConfigService::get 对字符串值会 json_decode，所以拿到的是**对象**。
	styleRaw, err := uc.cfg.GetDefault(ctx, "tabbar", "style", nil)
	if err != nil {
		return nil, err
	}
	res.Style = parseTabbarStyle(styleRaw)

	// 'login' => 6 项，全部走 config('project.login.*') 兜底
	// （x_config 无 login 段，ConfigService::get 会回退到 project.php）
	res.Login = LoginConfig{
		LoginWay:       []string{"1", "2"},
		CoerceMobile:   1,
		LoginAgreement: 1,
		ThirdAuth:      1,
		WechatAuth:     1,
		QqAuth:         0,
	}

	// 'website' => 3 项
	favicon, err := uc.cfg.GetDefault(ctx, "website", "h5_favicon", "")
	if err != nil {
		return nil, err
	}
	shopName, err := uc.cfg.GetDefault(ctx, "website", "shop_name", "")
	if err != nil {
		return nil, err
	}
	shopLogo, err := uc.cfg.GetDefault(ctx, "website", "shop_logo", "")
	if err != nil {
		return nil, err
	}
	res.Website = WebsiteConfig{
		H5Favicon: joinFileURL(requestDomain, toStr(favicon)),
		ShopName:  toStr(shopName),
		ShopLogo:  joinFileURL(requestDomain, toStr(shopLogo)),
	}

	// 'webPage' => 4 项，x_config 无 web_page 段 -> 全部走方法内默认值
	status, err := uc.cfg.GetDefault(ctx, "web_page", "status", 1)
	if err != nil {
		return nil, err
	}
	pageStatus, err := uc.cfg.GetDefault(ctx, "web_page", "page_status", 0)
	if err != nil {
		return nil, err
	}
	pageURL, err := uc.cfg.GetDefault(ctx, "web_page", "page_url", "")
	if err != nil {
		return nil, err
	}
	res.WebPage = WebPageConfig{
		Status:     int32(toIntAny(status, 1)),
		PageStatus: int32(toIntAny(pageStatus, 0)),
		PageURL:    toStr(pageURL),
		// 'url' => request()->domain() . '/mobile'
		URL: trimRightSlash(requestDomain) + "/mobile",
	}

	// 'copyright' => ConfigService::get('copyright','config',[]) -> 空数组
	res.Copyright = []any{}
	return res, nil
}

func parseTabbarStyle(v any) TabbarStyle {
	switch t := v.(type) {
	case nil:
		// 兜底：project.decorate.tabbar_style
		var s TabbarStyle
		_ = json.Unmarshal([]byte(DefaultTabbarStyleJSON), &s)
		return s
	case map[string]any:
		s := TabbarStyle{}
		if x, ok := t["default_color"].(string); ok {
			s.DefaultColor = x
		}
		if x, ok := t["selected_color"].(string); ok {
			s.SelectedColor = x
		}
		return s
	case string:
		var s TabbarStyle
		_ = json.Unmarshal([]byte(t), &s)
		return s
	default:
		var s TabbarStyle
		_ = json.Unmarshal([]byte(DefaultTabbarStyleJSON), &s)
		return s
	}
}

func toIntAny(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		n := 0
		for i := 0; i < len(t); i++ {
			if t[i] < '0' || t[i] > '9' {
				return def
			}
			n = n*10 + int(t[i]-'0')
		}
		return n
	default:
		return def
	}
}

// trimRightSlash 去掉**一个**尾部斜杠。
//
// ⚠️ 只去一个，对应 PHP 的 `substr_replace($domain,”,$len-1,1)`。
//
//	用 strings.TrimRight(s,"/") 会把连续的斜杠全去掉，对 "http://x//" 结果不同。
func trimRightSlash(s string) string {
	if len(s) > 0 && s[len(s)-1] == '/' {
		return s[:len(s)-1]
	}
	return s
}
