package biz

import (
	"encoding/json"
	"net/url"
	"testing"
)

// 分页参数的默认值和对齐点很容易抄错（尤其是 page_type 的默认值，
// 和 likeadmin 通用版本相反）。这里把边界钉死。

func TestParsePageParams_Defaults(t *testing.T) {
	p := ParsePageParams(url.Values{})
	// page_type 默认 1（分页），不是 0
	if p.PageType != 1 {
		t.Fatalf("page_type 默认应为 1，实际 %d", p.PageType)
	}
	if p.PageNo != 1 {
		t.Fatalf("page_no 默认应为 1，实际 %d", p.PageNo)
	}
	if p.PageSize != ListDefaultPageSize {
		t.Fatalf("page_size 默认应为 %d，实际 %d", ListDefaultPageSize, p.PageSize)
	}
	if p.Offset != 0 {
		t.Fatalf("offset 应为 0，实际 %d", p.Offset)
	}
	if p.Limit != ListDefaultPageSize {
		t.Fatalf("limit 应为 %d，实际 %d", ListDefaultPageSize, p.Limit)
	}
}

func TestParsePageParams_Paginated(t *testing.T) {
	p := ParsePageParams(url.Values{"page_no": {"3"}, "page_size": {"10"}})
	if p.PageNo != 3 || p.PageSize != 10 {
		t.Fatalf("pageNo=%d pageSize=%d", p.PageNo, p.PageSize)
	}
	// offset = (3-1)*10
	if p.Offset != 20 || p.Limit != 10 {
		t.Fatalf("offset=%d limit=%d", p.Offset, p.Limit)
	}
}

// PHP 用的是 `?:`，falsy（0、空串）都落回默认值
func TestParsePageParams_ZeroFallsBackToDefault(t *testing.T) {
	p := ParsePageParams(url.Values{"page_no": {"0"}, "page_size": {""}})
	if p.PageNo != 1 {
		t.Fatalf("page_no=0 应落回 1，实际 %d", p.PageNo)
	}
	if p.PageSize != ListDefaultPageSize {
		t.Fatalf("page_size 空应落回 %d，实际 %d", ListDefaultPageSize, p.PageSize)
	}
}

// 不分页模式：强制第一页 + 取最大条数
func TestParsePageParams_NotPaginated(t *testing.T) {
	p := ParsePageParams(url.Values{"page_type": {"0"}, "page_no": {"5"}, "page_size": {"10"}})
	if p.PageNo != 1 {
		t.Fatalf("不分页时 pageNo 应强制为 1，实际 %d", p.PageNo)
	}
	if p.PageSize != ListMaxPageSize {
		t.Fatalf("不分页时 pageSize 应为 %d，实际 %d", ListMaxPageSize, p.PageSize)
	}
	if p.Offset != 0 {
		t.Fatalf("不分页时 offset 应为 0，实际 %d", p.Offset)
	}
}

func TestPageParams_Float(t *testing.T) {
	p := ParsePageParams(url.Values{"price_start": {"12.5"}, "bad": {"abc"}})
	if got := p.Float("price_start"); got != 12.5 {
		t.Fatalf("price_start = %v", got)
	}
	if got := p.Float("bad"); got != 0 {
		t.Fatalf("非数字应返回 0，实际 %v", got)
	}
	if got := p.Float("missing"); got != 0 {
		t.Fatalf("缺失应返回 0，实际 %v", got)
	}
}

func TestPageParams_NonEmpty(t *testing.T) {
	p := ParsePageParams(url.Values{"keyword": {"abc"}, "empty": {""}, "zero": {"0"}})
	if got := p.NonEmpty("keyword", "def"); got != "abc" {
		t.Fatalf("keyword = %q", got)
	}
	// PHP 的 empty("") 为真 -> 取默认
	if got := p.NonEmpty("empty", "def"); got != "def" {
		t.Fatalf("空串应取默认，实际 %q", got)
	}
	// PHP 的 empty("0") 也为真 -> 取默认
	if got := p.NonEmpty("zero", "def"); got != "def" {
		t.Fatalf("\"0\" 应取默认（empty(\"0\") 为真），实际 %q", got)
	}
}

// dataLists 的 data 结构：键名必须 snake_case，extend 为空时是 [] 不是 null
func TestListData_ToJSON(t *testing.T) {
	d := ListData{
		Lists:    []any{map[string]any{"id": 1}},
		Count:    1,
		PageNo:   2,
		PageSize: 25,
	}
	raw, err := d.ToJSON()
	if err != nil {
		t.Fatalf("意外错误: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("输出不是合法 JSON: %v", err)
	}
	for _, k := range []string{"lists", "count", "page_no", "page_size", "extend"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("缺少键 %q，实际=%v", k, got)
		}
	}
	// 不能是 camelCase
	for _, bad := range []string{"pageNo", "pageSize"} {
		if _, ok := got[bad]; ok {
			t.Fatalf("出现了 camelCase 键 %q", bad)
		}
	}
	// extend 默认必须是空数组，不能是 null
	ext, ok := got["extend"].([]any)
	if !ok {
		t.Fatalf("extend 应为数组，实际 %T = %v", got["extend"], got["extend"])
	}
	if len(ext) != 0 {
		t.Fatalf("extend 应为空数组，实际 %v", ext)
	}
	// count 必须是数字
	if _, ok := got["count"].(float64); !ok {
		t.Fatalf("count 应为数字，实际 %T", got["count"])
	}
	// —— 值断言（不只是"键存在"）——
	//
	// 上面只检查了键是否出现。如果实现把 page_no 和 page_size 写反，
	// 那些断言照样通过。**presence 不等于 value** —— 这是本项目反复踩过
	// 的一类断言盲区（见第 17 轮那个 query 参数被忽略的 bug）。
	if got["count"] != float64(1) {
		t.Errorf("count 应为 1，实际 %#v", got["count"])
	}
	if got["page_no"] != float64(2) {
		t.Errorf("page_no 应为 2，实际 %#v", got["page_no"])
	}
	if got["page_size"] != float64(25) {
		t.Errorf("page_size 应为 25，实际 %#v", got["page_size"])
	}
	// 明确排除两者被写反的情况
	if got["page_no"] == got["page_size"] {
		t.Error("page_no 与 page_size 相等，无法区分是否写反 —— 请用不同的值构造用例")
	}
	// lists 为 nil 时要输出 []，不是 null
	empty, _ := ListData{PageNo: 1, PageSize: 25}.ToJSON()
	var got2 map[string]any
	_ = json.Unmarshal(empty, &got2)
	if _, ok := got2["lists"].([]any); !ok {
		t.Fatalf("lists 为空时应为 []，实际 %T = %v", got2["lists"], got2["lists"])
	}
}

func TestParseListTime(t *testing.T) {
	if _, ok := ParseListTime(""); ok {
		t.Fatal("空串应返回 false")
	}
	if _, ok := ParseListTime("not-a-time"); ok {
		t.Fatal("非法时间应返回 false")
	}
	for _, s := range []string{"2026-10-08 16:22:00", "2026-10-08"} {
		if _, ok := ParseListTime(s); !ok {
			t.Fatalf("%q 应能解析", s)
		}
	}
}
