package biz

import (
	"encoding/json"
	"testing"
	"time"
)

// PurchaseFaceRow.ToMap 的单测。
//
// 与 purchase_state_test.go 对称：那边补的是兑换单列表，这里补兑换专区列表。
// 两者都有"从 PHP 移植过来的宽容语义"，但**规则不同**，不能互相套用：
//
//	PurchaseFaceRow  : purchase_lists 假值 -> "--"，并连带把两个单价也变成 "--"
//	PurchaseStateRow : 没有 "--" 那套，只有库存兜底
//
// 本轮就是靠写这些用例，发现 purchase.go 里 issuer_time 用了 formatDBTime
// （NULL -> 空串），而 PHP 输出的是 null —— 见下面 TestFaceToMap_NullIssuerTimeIsNull。

func faceBaseRow() PurchaseFaceRow {
	return PurchaseFaceRow{
		ID:             3003,
		ArchiveID:      4004,
		Name:           "小磁守护者2号",
		Images:         strPtr(`["https://a/1.png"]`),
		Issuer:         "温州万昆鞋业有限公司",
		IssuerTime:     mkTime("2026-08-28 10:26:10"),
		PurchaseAmount: 0,
		LowUnitPrice:   strPtr("12.50"),
		MaxUnitPrice:   strPtr("99.90"),
		PurchaseLists:  nil,
		PlatformName:   "元梦典藏",
	}
}

func faceJSON(t *testing.T, m map[string]any) map[string]json.RawMessage {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	return got
}

// purchase_lists 为 nil -> "--"，且两个单价**也**变成 "--"
func TestFaceToMap_NilPurchaseListsBecomesDash(t *testing.T) {
	m := faceBaseRow().ToMap()
	if m["purchase_lists"] != "--" {
		t.Fatalf("purchase_lists 应为 \"--\"，实际 %#v", m["purchase_lists"])
	}
	if m["low_unit_price"] != "--" {
		t.Errorf("purchase_lists 为 \"--\" 时 low_unit_price 也应为 \"--\"，实际 %#v", m["low_unit_price"])
	}
	if m["max_unit_price"] != "--" {
		t.Errorf("purchase_lists 为 \"--\" 时 max_unit_price 也应为 \"--\"，实际 %#v", m["max_unit_price"])
	}
}

// purchase_lists 为 0 -> 同样是假值 -> "--"
func TestFaceToMap_ZeroPurchaseListsBecomesDash(t *testing.T) {
	r := faceBaseRow()
	zero := int64(0)
	r.PurchaseLists = &zero
	m := r.ToMap()
	if m["purchase_lists"] != "--" {
		t.Fatalf("0 是 PHP 假值，应变成 \"--\"，实际 %#v", m["purchase_lists"])
	}
	if m["low_unit_price"] != "--" || m["max_unit_price"] != "--" {
		t.Errorf("单价也应变成 \"--\"，实际 %#v / %#v", m["low_unit_price"], m["max_unit_price"])
	}
}

// purchase_lists 非 0 -> 保留为**数字**，两个单价显示真实值
func TestFaceToMap_NonZeroPurchaseListsKeepsPrices(t *testing.T) {
	r := faceBaseRow()
	n := int64(23)
	r.PurchaseLists = &n
	m := r.ToMap()

	got, ok := m["purchase_lists"].(int64)
	if !ok || got != 23 {
		t.Fatalf("purchase_lists 应为 int64(23)，实际 %T %#v", m["purchase_lists"], m["purchase_lists"])
	}
	if m["low_unit_price"] != "12.50" || m["max_unit_price"] != "99.90" {
		t.Fatalf("单价应显示真实值，实际 %#v / %#v", m["low_unit_price"], m["max_unit_price"])
	}
}

// ⚠️ 本项目里真实存在 NULL 的两行：x_app_archive.issuer_time 是 NULLABLE，
// 149 行里有 2 行为 NULL。PHP 的 toArray() 给 null，json_encode 出来是 null。
// 这里最初用的是 formatDBTime（NULL -> 空串），**是错的**，本用例把它钉住。
func TestFaceToMap_NullIssuerTimeIsNull(t *testing.T) {
	r := faceBaseRow()
	r.IssuerTime = nil
	got := faceJSON(t, r.ToMap())

	if string(got["issuer_time"]) != "null" {
		t.Fatalf("issuer_time 为 NULL 时应输出 JSON null，实际 %s（空串 %q 说明用了 formatDBTime）",
			got["issuer_time"], string(got["issuer_time"]))
	}
}

// images 为 NULL -> null（PHP 里 `is_string(null)` 为假，保持 NULL）
func TestFaceToMap_NullImagesIsNull(t *testing.T) {
	r := faceBaseRow()
	r.Images = nil
	got := faceJSON(t, r.ToMap())
	if string(got["images"]) != "null" {
		t.Fatalf("images 为 NULL 时应输出 null，实际 %s", got["images"])
	}
}

// images 非空 -> 数组
func TestFaceToMap_ImagesDecoded(t *testing.T) {
	m := faceBaseRow().ToMap()
	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 1 || imgs[0] != "https://a/1.png" {
		t.Fatalf("images 解码不对: %T %#v", m["images"], m["images"])
	}
}

// 键集合固定（11 个），且整数列是数字、金额列是数字
func TestFaceToMap_KeySet(t *testing.T) {
	m := faceBaseRow().ToMap()
	want := []string{
		"id", "archive_id", "name", "images", "issuer", "issuer_time",
		"purchase_amount", "low_unit_price", "max_unit_price",
		"purchase_lists", "platform_name",
	}
	if len(m) != len(want) {
		t.Fatalf("键数量 %d，期望 %d：%v", len(m), len(want), m)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("缺少键 %q", k)
		}
	}
	for _, k := range []string{"id", "archive_id", "purchase_amount"} {
		if _, ok := m[k].(int64); !ok {
			t.Errorf("%s 应为 int64（数字），实际 %T", k, m[k])
		}
	}
	// 单价是 decimal -> **字符串**（PHP 输出 "12.50"）
	for _, k := range []string{"low_unit_price", "max_unit_price"} {
		if _, ok := m[k].(string); !ok {
			t.Errorf("%s 应为 string（decimal 列 PHP 输出字符串），实际 %T", k, m[k])
		}
	}
}

// 时间格式必须是 PHP 的 'Y-m-d H:i:s'
func TestFaceToMap_TimeFormatIsPHP(t *testing.T) {
	got := faceJSON(t, faceBaseRow().ToMap())
	if string(got["issuer_time"]) != `"2026-08-28 10:26:10"` {
		t.Fatalf("issuer_time 格式不对: %s", got["issuer_time"])
	}
}

var _ = time.Now

// ⚠️ 本用例对应本轮发现的**潜在** bug：x_market_list_purchase.low_unit_price
// 是 decimal(10,2) **NULLABLE** —— 这几个价格列里唯一可空的一个
// （已核对 information_schema；max_unit_price / low_price / max_price /
//
//	unit_price 都是 NOT NULL）。
//
// 当前数据里该列为 NULL 的有 0 行，而且结果集里 3 行的 purchase_lists 全为 0
// （都走 "--" 分支），所以这条路径**根本没被走到**，是个还没发作的 bug。
//
// PHP 对 NULL 列输出 null；原先用 derefStr 会输出空串。
func TestFaceToMap_NullLowUnitPriceIsNull(t *testing.T) {
	r := faceBaseRow()
	n := int64(23) // 非 0，绕过 "--" 分支，让价格字段真正进入响应
	r.PurchaseLists = &n
	r.LowUnitPrice = nil
	r.MaxUnitPrice = nil

	got := faceJSON(t, r.ToMap())
	if string(got["low_unit_price"]) != "null" {
		t.Fatalf("low_unit_price 为 NULL 时应输出 JSON null，实际 %s（空串说明用了 derefStr）", got["low_unit_price"])
	}
	if string(got["max_unit_price"]) != "null" {
		t.Fatalf("max_unit_price 为 NULL 时应输出 JSON null，实际 %s", got["max_unit_price"])
	}
}
