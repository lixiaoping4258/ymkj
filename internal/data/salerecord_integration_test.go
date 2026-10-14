package data

import (
	"context"
	"testing"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// 集成测试：钉住 SaleRecordLists 的 **count 与 lists 口径不一致**（清单第 3 条，第 4 例）。
//
// 原实现：
//
//	lists():  4 张表 INNER JOIN —— sales_orders_detail + app_archive + market_sales + sales_orders
//	count():  只有 2 张表 INNER JOIN —— sales_orders_detail + app_archive
//
// 后果：count 会把被后两个 INNER JOIN 丢掉的行也算进去，**count >= lists 的实际行数**。
//
// 真实数据（已直查确认）：`x_sales_orders_detail` 有 2 行，user_id 都是 16；
// 其中 1 行的 `sale_id = 11` 在 `x_market_sales` 里**不存在**（孤儿），
// 所以 4 表口径只剩 1 行，而 2 表口径仍是 2 行。
//
// **这个测试的作用是防止"好心"把 count 也补上后两个 JOIN** ——
// 那样虽然更"一致"，但**改动了原项目的可观测行为**（分页总数会从 2 变成 1）。
func TestSaleRecord_CountAndListsUseDifferentJoins(t *testing.T) {
	db, prefix := openTestDB(t)
	repo := NewSaleRecordRepo(db, testConf(prefix))
	ctx := context.Background()

	q := biz.SaleRecordQuery{UserID: 16}

	cnt, err := repo.CountRecords(ctx, q)
	if err != nil {
		t.Fatalf("CountRecords 失败: %v", err)
	}
	rows, err := repo.ListRecords(ctx, q, biz.SaleRecordSort("time_desc"), 0, 50)
	if err != nil {
		t.Fatalf("ListRecords 失败: %v", err)
	}

	// 与直查的真实 SQL 对齐（不是与"期望的正确值"对齐 —— 原实现就是这样）
	var want2 int64
	if err := db.Raw("SELECT COUNT(*) FROM " + prefix + "sales_orders_detail d " +
		"INNER JOIN " + prefix + "app_archive a ON d.archive_id = a.id " +
		"WHERE d.user_id = 16").Scan(&want2).Error; err != nil {
		t.Fatalf("直查 2 表口径失败: %v", err)
	}
	var want4 int64
	if err := db.Raw("SELECT COUNT(*) FROM " + prefix + "sales_orders_detail d " +
		"INNER JOIN " + prefix + "app_archive a ON d.archive_id = a.id " +
		"INNER JOIN " + prefix + "market_sales m ON m.id = d.sale_id " +
		"INNER JOIN " + prefix + "sales_orders o ON o.id = d.order_id " +
		"WHERE d.user_id = 16").Scan(&want4).Error; err != nil {
		t.Fatalf("直查 4 表口径失败: %v", err)
	}

	if cnt != want2 {
		t.Errorf("CountRecords = %d，期望 %d（2 表口径）", cnt, want2)
	}
	if int64(len(rows)) != want4 {
		t.Errorf("ListRecords 返回 %d 行，期望 %d（4 表口径）", len(rows), want4)
	}

	// 核心断言：两者**本来就不相等** —— 相等说明有人"顺手修好"了，那是改动原逻辑
	if want2 == want4 {
		t.Skipf("当前数据下 2 表与 4 表口径恰好都是 %d 行，"+
			"无法体现差异（数据变了，测试仍然有效但失去区分力）", want2)
	}
	if cnt <= int64(len(rows)) {
		t.Errorf("原实现的 count(%d) 应当 **大于** lists 行数(%d) —— "+
			"若不再大于，说明 count 的 JOIN 被改动了（那是改动原项目逻辑）", cnt, len(rows))
	}
	t.Logf("count(2 表口径)=%d  >  lists 行数(4 表口径)=%d  ✔ 与原实现一致", cnt, len(rows))
}

// 钉住：SaleRecordSort 只有 time_* 两个分支，**没有 price 分支**。
//
// 同目录的 SaleOrderLists / SaleGoodsLists 都有 price_asc/price_desc，
// 而它们映射到的 `total_price` 列**在两张表里都不存在**（一用就 1054 报错）。
// SaleRecordLists **躲过了这个错** —— 不要"好心"给它补上 price 分支。
func TestSaleRecord_SortHasNoPriceBranch(t *testing.T) {
	cases := map[string]string{
		"time_asc":  "SalesOrdersDetail.start_time asc",
		"time_desc": "SalesOrdersDetail.start_time desc",
		// 未知值 / 其它列表用过的 price_* 都应落到 default
		"price_asc":  "SalesOrdersDetail.start_time desc",
		"price_desc": "SalesOrdersDetail.start_time desc",
		"bogus":      "SalesOrdersDetail.start_time desc",
		"":           "SalesOrdersDetail.start_time desc",
	}
	for in, want := range cases {
		if got := biz.SaleRecordSort(in); got != want {
			t.Errorf("SaleRecordSort(%q) = %q，期望 %q", in, got, want)
		}
	}
	// 明确断言：任何输入都不会产生含 total_price 的 ORDER BY
	for _, in := range []string{"price_asc", "price_desc", "time_asc", "time_desc", "x"} {
		if got := biz.SaleRecordSort(in); containsStr(got, "total_price") {
			t.Errorf("SaleRecordSort(%q) 产生了 total_price —— 该列不存在，会 1054 报错", in)
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
