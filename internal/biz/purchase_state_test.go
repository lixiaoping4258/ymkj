package biz

import (
	"encoding/json"
	"testing"
	"time"
)

// PurchaseStateRow.ToMap 的单测。
//
// 为什么需要：第 14 轮的集成测试覆盖的是 SQL 与计数，第 15 轮的接口验证只覆盖了
// **一个真实样本**。而 ToMap 里那几条从 PHP 移植过来的宽容语义，
// 恰好是最容易写错、也最难在集成测试里碰到的地方：
//
//   - `$stockNum == null` 在 PHP 里对 0 也为真 -> 库存 0 和缺键都走 bcsub 兜底
//   - images 为空串时是 `[]`（数组），不是 null
//   - integral 是**写在 if (is_string(images)) 里面**的，不是无条件设置
//   - grab_time / create_time 为 NULL 时是 null（不是空串）
//
// 这些用单元测试逐条钉住，比靠"碰巧取到某一行"可靠得多。

func mkTime(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		panic(err)
	}
	return &t
}

func strPtr(s string) *string { return &s }
func i32Ptr(v int32) *int32   { return &v }

// 构造一个字段都有值的行，各用例按需覆盖。
func baseRow() PurchaseStateRow {
	return PurchaseStateRow{
		ID:            1001,
		PayWay:        i32Ptr(1),
		ArchiveID:     2002,
		Amount:        5,
		ReceiveAmount: 2,
		UnitPrice:     strPtr("166.00"),
		CreateTime:    mkTime("2026-09-09 15:58:25"),
		GrabTime:      nil, // 该列可为 NULL
		Name:          "小磁守护者2号",
		Issuer:        "温州万昆鞋业有限公司",
		Images:        strPtr(`["https://a/1.png"]`),
		PlatformName:  "元梦典藏",
	}
}

// stockNum != 0 时用 Redis 的值（数字）
func TestToMap_StockNonZeroUsesRedisValue(t *testing.T) {
	m := baseRow().ToMap(7, true)
	got, ok := m["available_amount"].(int64)
	if !ok {
		t.Fatalf("available_amount 应为 int64，实际 %T = %#v", m["available_amount"], m["available_amount"])
	}
	if got != 7 {
		t.Fatalf("available_amount = %d, 期望 7", got)
	}
}

// ❗ 关键：库存恰好为 0 时，PHP 的 `0 == null` 为真 -> 也要走 bcsub 兜底。
// 写成 "exists ? num : fallback" 就会在这里返回数字 0，与 PHP 返回字符串 "3" 不同。
func TestToMap_StockZeroFallsBackLikePHP(t *testing.T) {
	m := baseRow().ToMap(0, true)
	got, ok := m["available_amount"].(string)
	if !ok {
		t.Fatalf("库存为 0 时应走 bcsub 兜底得到【字符串】，实际 %T = %#v",
			m["available_amount"], m["available_amount"])
	}
	// bcsub('5', '2', 0) -> "3"（注意没有小数位）
	if got != "3" {
		t.Fatalf("available_amount = %q, 期望 \"3\"", got)
	}
}

// 键不存在时同样走兜底
func TestToMap_StockMissingFallsBack(t *testing.T) {
	m := baseRow().ToMap(0, false)
	if got, ok := m["available_amount"].(string); !ok || got != "3" {
		t.Fatalf("缺键时应得到字符串 \"3\"，实际 %T = %#v", m["available_amount"], m["available_amount"])
	}
}

// images 非空 -> 解码成数组，且 integral 被设置
func TestToMap_ImagesDecodedAndIntegralSet(t *testing.T) {
	m := baseRow().ToMap(0, false)
	imgs, ok := m["images"].([]any)
	if !ok {
		t.Fatalf("images 应为数组，实际 %T", m["images"])
	}
	if len(imgs) != 1 || imgs[0] != "https://a/1.png" {
		t.Fatalf("images 内容不对: %#v", imgs)
	}
	// bcmul('5', '166.00', 2) -> "830.00"
	if got, ok := m["integral"].(string); !ok || got != "830.00" {
		t.Fatalf("integral = %#v, 期望字符串 \"830.00\"", m["integral"])
	}
}

// images 是空串 -> `[]`（数组），不是 null；**但 integral 仍会被设置**
// （原实现里 bcmul 在 if (is_string(images)) 之内，空串也是 string）
func TestToMap_EmptyImagesBecomesEmptyArray(t *testing.T) {
	r := baseRow()
	r.Images = strPtr("")
	m := r.ToMap(0, false)

	imgs, ok := m["images"].([]any)
	if !ok {
		t.Fatalf("空 images 应为数组，实际 %T = %#v", m["images"], m["images"])
	}
	if len(imgs) != 0 {
		t.Fatalf("空 images 应为空数组，实际 %#v", imgs)
	}
	if _, has := m["integral"]; !has {
		t.Fatal("images 是空串时 integral 仍应被设置（它在 is_string 分支内，空串也是 string）")
	}
}

// images 为 NULL -> null，且**不设置 integral**
func TestToMap_NullImagesSkipsIntegral(t *testing.T) {
	r := baseRow()
	r.Images = nil
	m := r.ToMap(0, false)

	if v, has := m["images"]; !has || v != nil {
		t.Fatalf("images 为 NULL 时应输出 null，实际 %#v (存在=%v)", v, has)
	}
	if _, has := m["integral"]; has {
		t.Fatal("images 为 NULL 时不应设置 integral（它在 if (is_string(images)) 分支内）")
	}
}

// 时间与可空列：NULL 必须是 JSON 的 null，不是空串。
//
// ⚠️ 这里断言的是**序列化后的 JSON**，不是 Go 侧的值。
// 第一版我写的是 `m["pay_way"] != nil`，结果失败了 —— 因为 pay_way 是
// `*int32(nil)`，装进 any 之后接口**不等于 nil**（带类型的 nil）。
// 但 json.Marshal 对 nil 指针输出 `null`，所以**契约是对的，是断言查错了层**。
//
// 契约是 JSON，断言就该查 JSON。
func TestToMap_NullableColumnsBecomeNull(t *testing.T) {
	r := baseRow()
	r.GrabTime = nil
	r.CreateTime = nil
	r.PayWay = nil
	r.UnitPrice = nil
	m := r.ToMap(0, false)

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	for _, k := range []string{"grab_time", "create_time", "pay_way"} {
		if string(got[k]) != "null" {
			t.Errorf("%s 应为 JSON null，实际 %s", k, got[k])
		}
	}
	// unit_price 为 nil -> JSON null（PHP 对 NULL 列给 null）
	if string(got["unit_price"]) != "null" {
		t.Errorf("unit_price 为 nil 时应输出 null，实际 %s", got["unit_price"])
	}
	// 对照：有值的整数列必须是数字，不能带引号
	if string(got["amount"]) != "5" {
		t.Errorf("amount 应为数字 5，实际 %s", got["amount"])
	}
	// 对照：available_amount 是 bcsub 的结果，是**字符串**
	if string(got["available_amount"]) != `"3"` {
		t.Errorf("available_amount 应为字符串 \"3\"，实际 %s", got["available_amount"])
	}
}

// 有值的时间必须是 PHP 的 'Y-m-d H:i:s'，不是 RFC3339
func TestToMap_TimeFormatIsPHPNotRFC3339(t *testing.T) {
	r := baseRow()
	r.GrabTime = mkTime("2026-10-08 09:30:00")
	m := r.ToMap(0, false)

	if m["create_time"] != "2026-09-09 15:58:25" {
		t.Errorf("create_time = %#v", m["create_time"])
	}
	if m["grab_time"] != "2026-10-08 09:30:00" {
		t.Errorf("grab_time = %#v（不是 RFC3339）", m["grab_time"])
	}
}

// 键集合必须固定 —— 多一个少一个都是静默的契约破坏
func TestToMap_KeySet(t *testing.T) {
	m := baseRow().ToMap(3, true)
	want := []string{
		"id", "pay_way", "archive_id", "amount", "receive_amount",
		"unit_price", "create_time", "grab_time",
		"name", "issuer", "platform_name",
		"available_amount", "images", "integral",
	}
	if len(m) != len(want) {
		t.Fatalf("键数量 %d，期望 %d：%v", len(m), len(want), m)
	}
	for _, k := range want {
		if _, ok := m[k]; !ok {
			t.Errorf("缺少键 %q", k)
		}
	}
	// 整数列必须是数字，不能变成字符串
	for _, k := range []string{"id", "archive_id", "amount", "receive_amount"} {
		if _, ok := m[k].(int64); !ok {
			t.Errorf("%s 应为 int64，实际 %T", k, m[k])
		}
	}
}
