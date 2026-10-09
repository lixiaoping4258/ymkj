package biz

import (
	"encoding/json"
	"testing"
	"time"
)

func ptrI64(v int64) *int64   { return &v }
func ptrStr(v string) *string { return &v }
func ptrTime(v string) *time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local)
	return &t
}

// ToMap 的 `"--"` 替换逻辑用**合成行**验证，不依赖库里的数据。
//
// ⚠️ 为什么要拆成单元测试：这几条断言原先写在 internal/data 的集成测试里，
// 对着 rows[0] 断言 `purchase_lists == "--"`。但那一行在库里**是会变的**
// （实测 purchase_lists 从 0 变成了 1），于是测试因为数据变化而失败 ——
// 看起来像代码坏了，其实是**把易变的数据当成了不变量**。
//
// 契约本身（原 PHP 的 `?:`）是稳定的：
//
//	'purchase_lists' => $v['purchase_lists'] ?: '--',
//	'low_unit_price' => $v['purchase_lists'] ? $v['low_unit_price'] : '--',
//	'max_unit_price' => $v['purchase_lists'] ? $v['max_unit_price'] : '--',
//
// **purchase_lists 为假值时，三个字段一起变 "--"；非假值时都保留原值。**
func TestPurchaseFaceRow_ToMap_DashSubstitution(t *testing.T) {
	base := func() PurchaseFaceRow {
		return PurchaseFaceRow{
			ID:           178056289901818,
			ArchiveID:    178091946400112,
			Name:         "马上元梦 · 元梦启程",
			Issuer:       "元梦空间数字科技（成都）有限公司",
			IssuerTime:   ptrTime("2026-04-17 17:39:39"),
			PlatformName: "元梦典藏",
			Images:       ptrStr(`[]`),
		}
	}

	t.Run("purchase_lists = 0 -> 三个字段都变 --", func(t *testing.T) {
		r := base()
		r.PurchaseLists = ptrI64(0)
		r.PurchaseAmount = 0
		r.LowUnitPrice = ptrStr("101.00")
		r.MaxUnitPrice = ptrStr("202.00")
		m := r.ToMap()

		if m["purchase_lists"] != "--" {
			t.Errorf("purchase_lists = %#v, 应为 \"--\"", m["purchase_lists"])
		}
		if m["low_unit_price"] != "--" {
			t.Errorf("low_unit_price = %#v, 应为 \"--\"", m["low_unit_price"])
		}
		if m["max_unit_price"] != "--" {
			t.Errorf("max_unit_price = %#v, 应为 \"--\"", m["max_unit_price"])
		}
	})

	t.Run("purchase_lists = 1 -> 三个字段保留原值", func(t *testing.T) {
		r := base()
		r.PurchaseLists = ptrI64(1)
		r.PurchaseAmount = 1
		r.LowUnitPrice = ptrStr("101.00")
		r.MaxUnitPrice = ptrStr("202.00")
		m := r.ToMap()

		if m["purchase_lists"] != int64(1) {
			t.Errorf("purchase_lists = %#v, 应为 1（非假值不替换）", m["purchase_lists"])
		}
		if m["low_unit_price"] != "101.00" {
			t.Errorf("low_unit_price = %#v, 应为 101.00", m["low_unit_price"])
		}
		if m["max_unit_price"] != "202.00" {
			t.Errorf("max_unit_price = %#v, 应为 202.00", m["max_unit_price"])
		}
	})

	t.Run("purchase_lists = NULL -> 也按假值变 --", func(t *testing.T) {
		r := base()
		r.PurchaseLists = nil
		r.LowUnitPrice = ptrStr("101.00")
		r.MaxUnitPrice = ptrStr("202.00")
		m := r.ToMap()
		if m["purchase_lists"] != "--" || m["low_unit_price"] != "--" || m["max_unit_price"] != "--" {
			t.Errorf("NULL 应按假值处理，实际 %#v / %#v / %#v",
				m["purchase_lists"], m["low_unit_price"], m["max_unit_price"])
		}
	})
}

// ToMap 的键集合与类型契约，同样用合成行（与库中数据无关）。
func TestPurchaseFaceRow_ToMap_Shape(t *testing.T) {
	r := PurchaseFaceRow{
		ID: 1, ArchiveID: 2, Name: "n", Issuer: "i",
		IssuerTime:     ptrTime("2026-04-17 17:39:39"),
		PurchaseLists:  ptrI64(1),
		PurchaseAmount: 3,
		LowUnitPrice:   ptrStr("1.00"),
		MaxUnitPrice:   ptrStr("2.00"),
		PlatformName:   "p",
		Images:         ptrStr(`["a.png","b.png"]`),
	}
	m := r.ToMap()

	want := []string{
		"id", "archive_id", "name", "issuer", "issuer_time",
		"purchase_lists", "purchase_amount", "low_unit_price",
		"max_unit_price", "platform_name", "images",
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("缺少键 %q（原契约是这 11 个 snake_case 键）", k)
		}
	}
	if len(m) != len(want) {
		t.Errorf("键数 = %d，应为 %d", len(m), len(want))
	}
	// images 必须是解码后的数组，不是原始 json 字符串
	if _, ok := m["images"].([]any); !ok {
		t.Errorf("images 应为数组，实际 %#v", m["images"])
	}
	// 时间必须是 PHP 的 'Y-m-d H:i:s'
	if m["issuer_time"] != "2026-04-17 17:39:39" {
		t.Errorf("issuer_time = %#v（应为 PHP 格式，不是 RFC3339）", m["issuer_time"])
	}
	// 契约以 JSON 为准：必须可序列化
	if _, err := json.Marshal(m); err != nil {
		t.Errorf("ToMap 结果无法 JSON 序列化: %v", err)
	}
}
