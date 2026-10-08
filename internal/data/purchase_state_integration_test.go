package data

import (
	"context"
	"testing"
	"time"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// 集成测试：验证 PurchaseStateLists 的等价查询与 PHP 一致。
//
// 期望值来源：PHP CLI 的 fetchSql(true) 抓到的真实 SQL（见 purchase_state.go 注释），
// 加上直查真实数据的计数。
//
// 覆盖点：
//  1. COMPLETE 分支的 state IN ('COMPLETE','OFFLINE') 与 receive_amount > 0
//  2. WANTED 分支没有 receive_amount 条件
//  3. **delete_time IS NULL 软删除过滤**（第六轮漏过这个，专门钉住）
//  4. 两个分支的终点计数与本测试里手写的等价 SQL 一致
func TestPurchaseStateRepo_MatchesRealSQL(t *testing.T) {
	db, prefix := openTestDB(t)
	repo := NewPurchaseStateRepo(db, testConf(prefix), nil, nil)
	ctx := context.Background()

	// --- FindListArchiveID ---
	// market_list_purchase.id = 1 的 archive_id 是 1（已直查确认）
	arcID, ok, err := repo.FindListArchiveID(ctx, 1)
	if err != nil {
		t.Fatalf("FindListArchiveID 失败: %v", err)
	}
	if !ok {
		t.Fatal("id=1 应存在")
	}
	var wantArc int64
	if err := db.Raw("SELECT archive_id FROM " + prefix + "market_list_purchase WHERE id = 1").
		Scan(&wantArc).Error; err != nil {
		t.Fatalf("直查失败: %v", err)
	}
	if arcID != wantArc {
		t.Fatalf("archive_id = %d, 直查 %d", arcID, wantArc)
	}

	// 不存在的 id -> (0, false, nil)，对应原实现 throw Exception('记录不存在')
	if _, ok, err := repo.FindListArchiveID(ctx, 999999999999); err != nil || ok {
		t.Fatalf("不存在的 id 应返回 (false, nil)，实际 ok=%v err=%v", ok, err)
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	// --- COMPLETE 分支 ---
	// 手写等价 SQL：state IN (COMPLETE, OFFLINE) AND end_time > now AND receive_amount > 0
	//                AND delete_time IS NULL
	// 注意 count 不 join app，但要 join app_archive（hasWhere 产生的 INNER JOIN）
	completeSQL := "SELECT COUNT(*) FROM " + prefix + "market_purchase AS MarketPurchase" +
		" INNER JOIN " + prefix + "app_archive AS AppArchive" +
		" ON MarketPurchase.archive_id = AppArchive.id" +
		" WHERE MarketPurchase.archive_id = ? AND MarketPurchase.end_time > ?" +
		" AND MarketPurchase.delete_time IS NULL" +
		" AND MarketPurchase.state IN ('COMPLETE','OFFLINE')" +
		" AND MarketPurchase.receive_amount > 0"
	var wantComplete int64
	if err := db.Raw(completeSQL, arcID, now).Scan(&wantComplete).Error; err != nil {
		t.Fatalf("直查 COMPLETE 失败: %v", err)
	}
	gotComplete, err := repo.Count(ctx, arcID, biz.MktPurchaseStateComplete)
	if err != nil {
		t.Fatalf("Count COMPLETE 失败: %v", err)
	}
	if gotComplete != wantComplete {
		t.Fatalf("COMPLETE count = %d, 等价 SQL = %d", gotComplete, wantComplete)
	}

	// --- 软删除过滤必须生效 ---
	// 找一个确实存在软删除记录的 archive，证明过滤没漏
	var anyArc int64
	softSQL := "SELECT archive_id FROM " + prefix + "market_purchase" +
		" WHERE delete_time IS NOT NULL AND state IN ('COMPLETE','OFFLINE') LIMIT 1"
	if err := db.Raw(softSQL).Scan(&anyArc).Error; err == nil && anyArc != 0 {
		withDeleted := "SELECT COUNT(*) FROM " + prefix + "market_purchase AS MarketPurchase" +
			" INNER JOIN " + prefix + "app_archive AS AppArchive" +
			" ON MarketPurchase.archive_id = AppArchive.id" +
			" WHERE MarketPurchase.archive_id = ? AND MarketPurchase.end_time > ?" +
			" AND MarketPurchase.state IN ('COMPLETE','OFFLINE')" +
			" AND MarketPurchase.receive_amount > 0"
		var raw int64
		if err := db.Raw(withDeleted, anyArc, now).Scan(&raw).Error; err != nil {
			t.Fatalf("直查(含软删除)失败: %v", err)
		}
		got, err := repo.Count(ctx, anyArc, biz.MktPurchaseStateComplete)
		if err != nil {
			t.Fatalf("Count 失败: %v", err)
		}
		if raw != got {
			t.Fatalf("archive=%d: 带软删除过滤应得 %d，实际 %d —— delete_time IS NULL 可能没生效",
				anyArc, raw, got)
		}
	}

	// --- WANTED 分支没有 receive_amount 条件 ---
	wantedSQL := "SELECT COUNT(*) FROM " + prefix + "market_purchase AS MarketPurchase" +
		" INNER JOIN " + prefix + "app_archive AS AppArchive" +
		" ON MarketPurchase.archive_id = AppArchive.id" +
		" WHERE MarketPurchase.archive_id = ? AND MarketPurchase.end_time > ?" +
		" AND MarketPurchase.delete_time IS NULL AND MarketPurchase.state = 'WANTED'"
	var wantWanted int64
	if err := db.Raw(wantedSQL, arcID, now).Scan(&wantWanted).Error; err != nil {
		t.Fatalf("直查 WANTED 失败: %v", err)
	}
	gotWanted, err := repo.Count(ctx, arcID, biz.MktPurchaseStateWanted)
	if err != nil {
		t.Fatalf("Count WANTED 失败: %v", err)
	}
	if gotWanted != wantWanted {
		t.Fatalf("WANTED count = %d, 等价 SQL = %d", gotWanted, wantWanted)
	}

	// --- List 能跑，字段类型对 ---
	rows, err := repo.List(ctx, arcID, biz.MktPurchaseStateComplete, 0, 5)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	for _, r := range rows {
		if r.ID == 0 {
			t.Error("行 id 不应为 0")
		}
		// unit_price 是 decimal，必须扫成字符串（PHP 输出 "166.00" 这种）
		if r.UnitPrice != nil && len(*r.UnitPrice) == 0 {
			t.Error("unit_price 不应为空串")
		}
	}

	// 排序：COMPLETE 分支是 id DESC，验证首行 id 最大
	if len(rows) >= 2 && rows[0].ID < rows[1].ID {
		t.Errorf("COMPLETE 应按 id DESC，实际首两行 %d < %d", rows[0].ID, rows[1].ID)
	}
}
