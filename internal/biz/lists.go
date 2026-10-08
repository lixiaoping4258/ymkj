package biz

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 本文件对应原项目 app/common/lists/BaseDataLists.php 与 JsonService::dataLists。
//
// 为什么值得单独做一层：全项目有 **77 个列表类**（39 adminapi + 29 api + 9 common）
// 都继承这套分页/搜索/排序/导出的模式。把这层做对，后面每个列表接口都是照抄。

// 分页默认值，取自 config/project.php 的 lists 段。
const (
	ListDefaultPageSize = 25
	ListMaxPageSize     = 1000000
	ListDefaultPageNo   = 1

	// ListPageTypePaginated 正常分页；其它值 = 不分页，一次取最大条数
	ListPageTypePaginated = 1
)

// PageParams 是列表接口的分页参数。
type PageParams struct {
	PageNo   int
	PageSize int
	// PageType: 1=正常分页；0=不分页，一次取最大条数
	PageType int
	Offset   int
	Limit    int
	// 原始查询参数，供各列表自己取 keyword / price_start / sort 这类业务字段
	Query url.Values
}

// ParsePageParams 逐行对照 BaseDataLists::initPage。
//
// 三个容易写错的地方：
//  1. **page_type 默认是 1（分页）**，不是 0。
//     这一点和 likeadmin 通用版本相反，照抄别处的实现会得到「一次返回全表」。
//  2. **page_type 是「参数缺失才取默认」，不是 `?:`**：
//     PHP 写的是 `(int)$request->get('page_type', 1)`，
//     显式传 `?page_type=0` 就是 0（不分页），不能被当成缺失去落回 1。
//     搞错的后果是「调用方请求全量数据只拿到 25 条」——这个坑是单测抓出来的。
//  3. page_no / page_size 用的才是 `?:`（falsy 就取默认），
//     所以 page_no=0、page_size="" 都要落回默认值。
func ParsePageParams(q url.Values) PageParams {
	pageType := ListPageTypePaginated
	if vs, present := q["page_type"]; present {
		v := ""
		if len(vs) > 0 {
			v = vs[0]
		}
		// 显式传了就按传的算；空串按 0 处理，与 PHP 的 (int)"" 一致
		pageType = phpInt(v)
	}

	var pageNo, pageSize int
	if pageType == ListPageTypePaginated {
		pageNo = phpInt(q.Get("page_no"))
		if pageNo == 0 {
			pageNo = ListDefaultPageNo
		}
		pageSize = phpInt(q.Get("page_size"))
		if pageSize == 0 {
			pageSize = ListDefaultPageSize
		}
	} else {
		// 不分页：强制第一页，直接取最大记录数
		pageNo = 1
		pageSize = ListMaxPageSize
	}

	return PageParams{
		PageNo:   pageNo,
		PageSize: pageSize,
		PageType: pageType,
		Offset:   (pageNo - 1) * pageSize,
		Limit:    pageSize,
		Query:    q,
	}
}

// Str 取字符串参数（对应 $this->params['x'] ?? ”）。
func (p PageParams) Str(key string) string {
	return p.Query.Get(key)
}

// NonEmpty 对应 PHP 的 !empty($this->params[$key])，为空则返回 default。
func (p PageParams) NonEmpty(key, def string) string {
	if PhpTruthy(p.Query.Get(key)) {
		return p.Query.Get(key)
	}
	return def
}

// Float 对应 floatval($this->params[$key])，非数字返回 0。
func (p PageParams) Float(key string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(p.Query.Get(key)), 64)
	if err != nil {
		return 0
	}
	return f
}

// ListData 对应 JsonService::dataLists 里组装出来的 data。
//
// 序列化后的键名必须是 snake_case（page_no / page_size），
// 因为原 PHP 就是这么写的数组键。所以这里用 map 而不是 struct tag ——
// 显式、不会因为改字段名而悄悄变键名。
type ListData struct {
	Lists    any
	Count    int64
	PageNo   int
	PageSize int
	// Extend 对应 ListsExtendInterface::extend()，多数接口没有，默认空数组
	Extend any
}

// ToJSON 输出原项目的 data 形态：
//
//	{"lists":[...],"count":N,"page_no":1,"page_size":25,"extend":[]}
//
// 注意 extend 为空时是 **[]** 而不是 null —— PHP 那边是 $data['extend'] = []。
func (d ListData) ToJSON() (json.RawMessage, error) {
	lists := d.Lists
	if lists == nil {
		lists = []any{}
	}
	extend := d.Extend
	if extend == nil {
		extend = []any{}
	}
	m := map[string]any{
		"lists":     lists,
		"count":     d.Count,
		"page_no":   d.PageNo,
		"page_size": d.PageSize,
		"extend":    extend,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// phpInt 对应 PHP 的 (int) 转换：非法值当 0。
func phpInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		// PHP (int)"12abc" == 12，这里退化处理即可，分页参数不会有这种值
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return 0
		}
		return int(f)
	}
	return n
}

// 供各列表实现参考的时间解析：原项目 start_time/end_time 走 strtotime。
const listTimeLayout = "2006-01-02 15:04:05"

// ParseListTime 对应 strtotime()，失败返回零值与 false。
func ParseListTime(s string) (time.Time, bool) {
	if !PhpTruthy(s) {
		return time.Time{}, false
	}
	for _, layout := range []string{listTimeLayout, "2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
