package biz

import (
	"encoding/json"
	"testing"
)

// SaleFaceRow.ToMap 的单测。
//
// 这是本项目第三个 ToMap，三个的规则**各不相同**，不能互相套用：
//
//	PurchaseFaceRow  : purchase_lists 假值 -> "--"，并连带把两个单价也变成 "--"
//	PurchaseStateRow : 没有 "--" 那套，只有库存兜底
//	SaleFaceRow      : **只有 images 的 json 解码**，没有任何 "--" 替换
//
// 本轮靠"把已验证的判断标准套到同类地方"发现了第二处同样的 bug：
// sale.go 的 issuer_time 也用了 formatDBTime（NULL -> 空串）。
// 修完后那个函数已从代码里删除。

func saleBaseRow() SaleFaceRow {
	return SaleFaceRow{
		ID:           5005,
		Name:         "小磁守护者2号",
		Images:       strPtr(`["https://a/1.png"]`),
		Issuer:       "温州万昆鞋业有限公司",
		IssuerTime:   mkTime("2026-08-28 10:26:10"),
		SaleAmount:   12,
		LowPrice:     strPtr("12.50"),
		MaxPrice:     strPtr("99.90"),
		PlatformName: "元梦典藏",
	}
}

func saleJSON(t *testing.T, m map[string]any) map[string]json.RawMessage {
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

// ⚠️ 本用例对应的 bug：x_app_archive.issuer_time 是 datetime NULLABLE
// （实测 149 行里 2 行为 NULL），PHP 输出 null，而这里最初用 formatDBTime
// 输出空串。与 purchase.go 是同一处错误、不同文件。
func TestSaleToMap_NullIssuerTimeIsNull(t *testing.T) {
	r := saleBaseRow()
	r.IssuerTime = nil
	got := saleJSON(t, r.ToMap())
	if string(got["issuer_time"]) != "null" {
		t.Fatalf("issuer_time 为 NULL 时应输出 JSON null，实际 %s", got["issuer_time"])
	}
}

// 与 PurchaseFaceRow 的关键区别：SaleFaceRow **没有 "--" 那套替换**。
// 即使价格字段为 nil，也输出空串而不是 "--"。
func TestSaleToMap_NoDashSubstitution(t *testing.T) {
	r := saleBaseRow()
	r.LowPrice = nil
	r.MaxPrice = nil
	m := r.ToMap()

	if m["low_price"] == "--" || m["max_price"] == "--" {
		t.Fatalf("SaleFaceRow 不应有 \"--\" 替换（那是 PurchaseFaceRow 的规则），实际 %#v / %#v",
			m["low_price"], m["max_price"])
	}
	got := saleJSON(t, r.ToMap())
	if string(got["low_price"]) != "null" || string(got["max_price"]) != "null" {
		t.Fatalf("价格为 nil 时应输出 JSON null（PHP 对 NULL 列给 null），实际 %s / %s", got["low_price"], got["max_price"])
	}
	// 也不应该有 purchase_lists / integral 之类的键
	for _, k := range []string{"purchase_lists", "integral", "max_unit_price"} {
		if _, has := m[k]; has {
			t.Errorf("不应存在键 %q（那是别的 ToMap 的字段）", k)
		}
	}
}

func TestSaleToMap_NullImagesIsNull(t *testing.T) {
	r := saleBaseRow()
	r.Images = nil
	if string(saleJSON(t, r.ToMap())["images"]) != "null" {
		t.Fatal("images 为 NULL 时应输出 null")
	}
}

func TestSaleToMap_ImagesDecoded(t *testing.T) {
	m := saleBaseRow().ToMap()
	imgs, ok := m["images"].([]any)
	if !ok || len(imgs) != 1 || imgs[0] != "https://a/1.png" {
		t.Fatalf("images 解码不对: %T %#v", m["images"], m["images"])
	}
}

func TestSaleToMap_KeySet(t *testing.T) {
	m := saleBaseRow().ToMap()
	want := []string{
		"id", "name", "images", "issuer", "issuer_time",
		"sale_amount", "low_price", "max_price", "platform_name",
	}
	if len(m) != len(want) {
		t.Fatalf("键数量 %d，期望 %d：%v", len(m), len(want), m)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("缺少键 %q", k)
		}
	}
}

func TestSaleToMap_TypesAndTimeFormat(t *testing.T) {
	got := saleJSON(t, saleBaseRow().ToMap())

	// 整数列 -> JSON 数字（不带引号）
	if string(got["id"]) != "5005" {
		t.Errorf("id 应为数字 5005，实际 %s", got["id"])
	}
	if string(got["sale_amount"]) != "12" {
		t.Errorf("sale_amount 应为数字 12，实际 %s", got["sale_amount"])
	}
	// decimal 列 -> **字符串**（带引号），这是 PHP 的输出形态
	if string(got["low_price"]) != `"12.50"` {
		t.Errorf("low_price 应为字符串 \"12.50\"，实际 %s", got["low_price"])
	}
	if string(got["max_price"]) != `"99.90"` {
		t.Errorf("max_price 应为字符串 \"99.90\"，实际 %s", got["max_price"])
	}
	// 时间是 PHP 的 'Y-m-d H:i:s'，不是 RFC3339
	if string(got["issuer_time"]) != `"2026-08-28 10:26:10"` {
		t.Errorf("issuer_time 格式不对: %s", got["issuer_time"])
	}
}
