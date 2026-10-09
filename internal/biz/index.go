package biz

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// IndexUsecase 对应 app/api/logic/IndexLogic.php 与 GetListLogic::getList。
//
// 本批实现 3 条（原文 index.php 的 5 条中）：
//   - GetDecorate      -> IndexLogic::getDecorate
//   - GetBannerList    -> GetListLogic::getList
//   - GetIndex         -> IndexLogic::getIndexData
//
// 未做：GetConfig（IndexLogic::getConfigData）、Test（这个返回体结构特殊，见文档）。
type IndexUsecase struct {
	repo  IndexRepo
	cache RawKeyCache
}

func NewIndexUsecase(repo IndexRepo, cache RawKeyCache) *IndexUsecase {
	return &IndexUsecase{repo: repo, cache: cache}
}

// IndexRepo 装修页 / banner / 首页文章。
type IndexRepo interface {
	// FindDecoratePage 对应：
	//   DecoratePage::field(['type','name','data','meta'])->findOrEmpty($id)->toArray()
	//
	// ⚠️ x_decorate_page **没有 delete_time 列**，所以没有软删除过滤。
	// ⚠️ data / meta 是 text 列，模型**没有 json 类型转换**（DecoratePage 是空类），
	//    所以 PHP 输出的是**原始字符串**，不是数组 —— 这里也用 string。
	FindDecoratePage(ctx context.Context, id int32) (*DecoratePageRow, error)

	// ListBanners 对应 GetListLogic::getList 的查询。
	ListBanners(ctx context.Context) ([]BannerRow, error)

	// ListArticles 对应 getIndexData 里的文章查询。
	ListArticles(ctx context.Context, limit int) ([]ArticleRow, error)
}

// DecoratePageRow 对应 field(['type','name','data','meta'])。
type DecoratePageRow struct {
	Type int32
	Name string
	Data string
	Meta string
}

// BannerRow 对应 field(['id','title','image','jump_url','sort','create_time'])。
type BannerRow struct {
	ID         int64
	Title      string
	Image      string
	JumpURL    string
	Sort       int32
	CreateTime string // datetime -> PHP 的 'Y-m-d H:i:s' 字符串
}

// ArticleRow 对应 getIndexData 里的文章字段。
//
// ⚠️ 原文 select 了 click_actual / click_virtual，然后 `->hidden([...])` 把它们**藏掉**，
// 同时 `->append(['click'])` 加上计算列 click = click_actual + click_virtual。
// 所以最终对外只有 8 个键：id / title / desc / abstract / image / author /
// create_time / **click**。
type ArticleRow struct {
	ID         int64
	Title      string
	Desc       string
	Abstract   string
	Image      string
	Author     string
	CreateTime int64 // x_article.create_time 是 int(11)
	Click      int64 // = click_actual + click_virtual
}

// IndexDataResult 对应 getIndexData 的 `['page'=>..., 'article'=>...]`。
type IndexDataResult struct {
	Page    *DecoratePageRow
	Article []ArticleRow
}

// IndexArticleLimit 对应 `->limit(20)`。
const IndexArticleLimit = 20

// BannerCacheKey 对应 GetListLogic 里的 `'banner:list'`。
//
// ⚠️ 这个键走的是 **RedisLockService（裸 phpredis，无 TP 前缀）**，
// 也就是**与 PHP 共用**的键（见 README 第七节第 8 条：缓存隔离、锁共用）。
// 所以这里用 RawKeyCache 而不是 Cache。
const BannerCacheKey = "banner:list"

// BannerCacheTTL 对应 `RedisLockService::set($cacheKey, json_encode($lists), 10)`。
const BannerCacheTTL = 10 * time.Second

// GetDecorate 逐字对应 IndexLogic::getDecorate($id)。
func (uc *IndexUsecase) GetDecorate(ctx context.Context, id int32) (*DecoratePageRow, error) {
	return uc.repo.FindDecoratePage(ctx, id)
}

// GetBannerList 逐字对应 GetListLogic::getList()。
//
// 原文：
//
//	$cacheKey = 'banner:list';
//	$ret = RedisLockService::get($cacheKey);
//	if($ret) { return json_decode($ret, true); }        // 命中缓存直接返回
//	$lists = Db::name('banner')->where('is_show',1)->whereNull('delete_time')
//	    ->order('sort','desc')->order('create_time','desc')
//	    ->field([...])->select()->toArray();
//	foreach ($lists as &$item) { if(!empty($item['image'])) $item['image'] = get_image_url($item['image']); }
//	RedisLockService::set($cacheKey, json_encode($lists), 10);
//	return $lists;
//
// ⚠️ 缓存命中时返回的是缓存里的内容（可能来自 PHP），所以这里读缓存必须
//
//	用**与 PHP 相同的键**（无隔离前缀）。
func (uc *IndexUsecase) GetBannerList(ctx context.Context) ([]BannerRow, error) {
	// ⚠️ RawKeyCache.Get 命中时返回 string（值就是 PHP 的 json_encode 结果），
	//    未命中返回 nil。形状必须与 PHP 逐字一致，否则解析失败要按未命中处理。
	if v, err := uc.cache.Get(ctx, BannerCacheKey); err != nil {
		// 读缓存失败不致命：原文 RedisLockService::get 出错也走回源
		_ = err
	} else if v != nil {
		if s, ok := v.(string); ok && s != "" {
			var cached []BannerRow
			if json.Unmarshal([]byte(s), &cached) == nil {
				return cached, nil
			}
			// 解析失败按未命中处理：原文 json_decode 失败返回 null，
			// 而 `if($ret)` 对 null 为假 -> 同样走回源。行为一致。
		}
	}

	lists, err := uc.repo.ListBanners(ctx)
	if err != nil {
		return nil, err
	}
	// 格式化图片 URL（对应 get_image_url）
	for i := range lists {
		if lists[i].Image != "" {
			lists[i].Image = GetImageURL(lists[i].Image, uc.imagePrefix(ctx))
		}
	}
	if b, err := json.Marshal(lists); err == nil {
		// 写缓存失败不影响返回（原文 RedisLockService::set 也没接返回值）
		_ = uc.cache.Set(ctx, BannerCacheKey, b, BannerCacheTTL)
	}
	return lists, nil
}

// GetIndex 逐字对应 IndexLogic::getIndexData()。
func (uc *IndexUsecase) GetIndex(ctx context.Context) (*IndexDataResult, error) {
	page, err := uc.repo.FindDecoratePage(ctx, 1) // DecoratePage::findOrEmpty(1)
	if err != nil {
		return nil, err
	}
	articles, err := uc.repo.ListArticles(ctx, IndexArticleLimit)
	if err != nil {
		return nil, err
	}
	return &IndexDataResult{Page: page, Article: articles}, nil
}

// imagePrefix 由 handler 注入（对应 ConfigService::get('website','shop_logo') 那类
// 需要拼域名的场景，以及 get_image_url 的域名来源）。这里先留空，
// 因为实测 banner 的 image 都是完整 URL，get_image_url 会原样返回。
func (uc *IndexUsecase) imagePrefix(_ context.Context) string { return "" }

// GetImageURL 对应 app/common.php 的 get_image_url($imagePath)。
//
// 已读到的部分：
//
//	if (empty($imagePath)) { return ''; }
//	if (strpos($imagePath,'http://') === 0 || strpos($imagePath,'https://') === 0) { return $imagePath; }
//	// 拼接完整URL ...
//
// ⚠️ 后半段（拼接时用哪个域名）**没有读完**。实测库里 banner 的 image
//
//	全都是完整 URL（https://test-xmarket.tsl3060.com/...），所以走的是
//	原样返回分支，**当前实现路径不经过拼接**。
//	等真的遇到相对路径的图片时，必须先把 get_image_url 的后半段读完再对齐 ——
//	这里先按"去尾斜杠 + 拼前缀"实现并标注为待确认。
func GetImageURL(imagePath, prefix string) string {
	if imagePath == "" {
		return ""
	}
	if strings.HasPrefix(imagePath, "http://") || strings.HasPrefix(imagePath, "https://") {
		return imagePath
	}
	if prefix == "" {
		return imagePath
	}
	return strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(imagePath, "/")
}
