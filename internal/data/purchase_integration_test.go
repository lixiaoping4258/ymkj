package data

import (
	"context"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/lixiaoping4258/ymkj/internal/biz"
	"github.com/lixiaoping4258/ymkj/internal/conf"
)

// 集成测试：连真实数据库跑 PurchaseFaceLists 的等价查询，
// 与用 PHP 跑同一份代码得到的结果比对。
//
// 期望值是**实测出来的**，不是推断的：用 PHP CLI 启动 ThinkPHP，
// 直接 new PurchaseFaceLists() 并 json_encode 得到。原样贴在下面。
//
// 需要仓库根有 configs/config.local.yaml（含真实凭据、已 gitignore），
// 没有就跳过 —— 保证 CI 里不会因为缺配置而红。
func TestPurchaseFaceRepo_MatchesRealPHP(t *testing.T) {
	root := findRepoRoot(t)
	cfgPath := root + "/configs/config.local.yaml"
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("configs/config.local.yaml 不存在，跳过集成测试")
	}

	var cfg struct {
		Data struct {
			Database struct {
				Source string `yaml:"source"`
				Prefix string `yaml:"prefix"`
			} `yaml:"database"`
		} `yaml:"data"`
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("读配置失败: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	db, err := gorm.Open(mysql.Open(cfg.Data.Database.Source), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.Data.Database.Prefix,
			SingularTable: true,
		},
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("连库失败: %v", err)
	}

	repo := NewPurchaseFaceRepo(db, testConf(cfg.Data.Database.Prefix))
	ctx := context.Background()

	// 与 PHP 探测时相同的分页参数
	page := biz.ParsePageParams(nil)
	page.PageNo, page.PageSize, page.Offset, page.Limit = 1, 2, 0, 2
	q := biz.PurchaseFaceQuery{Page: page}

	count, err := repo.Count(ctx, q, false)
	if err != nil {
		t.Fatalf("Count 失败: %v", err)
	}
	// PHP: count=3（44 条 list_purchase 里只有 3 条的 archive 满足 state=1）
	if count != 3 {
		t.Fatalf("count 应为 3（与 PHP 探测结果一致），实际 %d", count)
	}

	rows, err := repo.List(ctx, q, 0, 2, false)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应返回 2 行，实际 %d", len(rows))
	}

	first := rows[0].ToMap()

	// ---- 下面这些期望值全部来自真实 PHP 的输出 ----
	// {"id":178056289901818,"archive_id":178091946400112,"name":"马上元梦 · 元梦启程",
	//  "issuer":"元梦空间数字科技（成都）有限公司","issuer_time":"2026-04-17 17:39:39",
	//  "purchase_amount":0,"low_unit_price":"--","max_unit_price":"--",
	//  "purchase_lists":"--","platform_name":"元梦典藏"}
	if first["id"] != int64(178056289901818) {
		t.Errorf("id = %#v, PHP 是 178056289901818", first["id"])
	}
	if first["archive_id"] != int64(178091946400112) {
		t.Errorf("archive_id = %#v", first["archive_id"])
	}
	if first["name"] != "马上元梦 · 元梦启程" {
		t.Errorf("name = %#v", first["name"])
	}
	if first["issuer"] != "元梦空间数字科技（成都）有限公司" {
		t.Errorf("issuer = %#v", first["issuer"])
	}
	// 时间必须是 PHP 的 'Y-m-d H:i:s'，不能是 Go 的 RFC3339
	if first["issuer_time"] != "2026-04-17 17:39:39" {
		t.Errorf("issuer_time = %#v（应为 2026-04-17 17:39:39，不是 RFC3339）", first["issuer_time"])
	}
	if first["purchase_amount"] != int64(0) {
		t.Errorf("purchase_amount = %#v", first["purchase_amount"])
	}
	// purchase_lists 在 DB 里是 0，PHP 用 ?: 把它变成 "--"，并连带把两个单价也变成 "--"
	if first["purchase_lists"] != "--" {
		t.Errorf("purchase_lists = %#v, 应为 \"--\"", first["purchase_lists"])
	}
	if first["low_unit_price"] != "--" {
		t.Errorf("low_unit_price = %#v, 应为 \"--\"", first["low_unit_price"])
	}
	if first["max_unit_price"] != "--" {
		t.Errorf("max_unit_price = %#v, 应为 \"--\"", first["max_unit_price"])
	}
	if first["platform_name"] != "元梦典藏" {
		t.Errorf("platform_name = %#v", first["platform_name"])
	}
	// images 在 DB 里是 json 字符串，必须被解码成数组
	imgs, ok := first["images"].([]any)
	if !ok {
		t.Fatalf("images 应为数组，实际 %T = %#v", first["images"], first["images"])
	}
	if len(imgs) != 2 {
		t.Errorf("images 应有 2 个元素，实际 %d", len(imgs))
	}

	// 键集合必须与 PHP 完全一致（多一个少一个都是静默的契约破坏）
	wantKeys := []string{
		"id", "archive_id", "name", "images", "issuer", "issuer_time",
		"purchase_amount", "low_unit_price", "max_unit_price",
		"purchase_lists", "platform_name",
	}
	if len(first) != len(wantKeys) {
		t.Errorf("键数量 %d，应为 %d：%v", len(first), len(wantKeys), first)
	}
	for _, k := range wantKeys {
		if _, ok := first[k]; !ok {
			t.Errorf("缺少键 %q", k)
		}
	}
}

// TestPurchaseFaceRepo_SortVariants 验证 5 种排序都能生成合法 SQL 并返回结果。
// 排序字段名只从白名单映射，不会拼接用户输入。
func TestPurchaseFaceRepo_SortVariants(t *testing.T) {
	root := findRepoRoot(t)
	cfgPath := root + "/configs/config.local.yaml"
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("configs/config.local.yaml 不存在，跳过集成测试")
	}
	var cfg struct {
		Data struct {
			Database struct {
				Source string `yaml:"source"`
				Prefix string `yaml:"prefix"`
			} `yaml:"database"`
		} `yaml:"data"`
	}
	raw, _ := os.ReadFile(cfgPath)
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	db, err := gorm.Open(mysql.Open(cfg.Data.Database.Source), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.Data.Database.Prefix,
			SingularTable: true,
		},
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("连库失败: %v", err)
	}
	repo := NewPurchaseFaceRepo(db, testConf(cfg.Data.Database.Prefix))

	// 故意带一个注入尝试：orderBy 走白名单，应落回 default
	for _, s := range []string{"low", "high", "public", "done", "unknown", "purchase_lists desc; DROP TABLE x_user"} {
		page := biz.PageParams{PageNo: 1, PageSize: 2, Offset: 0, Limit: 2}
		q := biz.PurchaseFaceQuery{Page: page, Sort: s}
		if _, err := repo.List(context.Background(), q, 0, 2, false); err != nil {
			t.Fatalf("sort=%q 查询失败: %v", s, err)
		}
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("取工作目录失败: %v", err)
	}
	// internal/data -> 项目根
	return dir + "/../.."
}

// TestSaleFaceRepo_MatchesRealPHP 验证秒转列表的查询与 PHP 一致。
//
// 这个接口的行级验证做不了：x_market_list_sales 只有 4 行，且与 state=1 的
// archive join 后是 **0 行**（已直查确认）。PHP 跑出来也是 count=0 / rows=0。
// 所以这里断言的是：count 一致、SQL 能跑、5 种排序都安全。
//
// 顺带锁住一个容易抄错的差异：**SaleFaceLists 的 switch($sort) 没有 default**，
// 所以不带 sort 时 SQL 里根本没有 ORDER BY；而 PurchaseFaceLists 有 default。
func TestSaleFaceRepo_MatchesRealPHP(t *testing.T) {
	root := findRepoRoot(t)
	cfgPath := root + "/configs/config.local.yaml"
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("configs/config.local.yaml 不存在，跳过集成测试")
	}
	var cfg struct {
		Data struct {
			Database struct {
				Source string `yaml:"source"`
				Prefix string `yaml:"prefix"`
			} `yaml:"database"`
		} `yaml:"data"`
	}
	raw, _ := os.ReadFile(cfgPath)
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	db, err := gorm.Open(mysql.Open(cfg.Data.Database.Source), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.Data.Database.Prefix,
			SingularTable: true,
		},
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("连库失败: %v", err)
	}
	repo := NewSaleFaceRepo(db, testConf(cfg.Data.Database.Prefix))
	ctx := context.Background()

	page := biz.PageParams{PageNo: 1, PageSize: 2, Offset: 0, Limit: 2}
	q := biz.SaleFaceQuery{Page: page}

	// PHP 实测结果：count=0
	count, err := repo.Count(ctx, q)
	if err != nil {
		t.Fatalf("Count 失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("count 应为 0（与 PHP 实测一致：list_sales 4 行但 join state=1 后为 0），实际 %d", count)
	}
	rows, err := repo.List(ctx, q, 0, 2)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("应返回 0 行，实际 %d", len(rows))
	}

	// 5 种排序（含不带 sort 的"无 ORDER BY"分支）都必须能跑
	for _, s := range []string{"", "low", "high", "public", "done", "bogus", "x; DROP TABLE x_user"} {
		qq := biz.SaleFaceQuery{Page: page, Sort: s}
		if _, err := repo.List(ctx, qq, 0, 2); err != nil {
			t.Fatalf("sort=%q 查询失败: %v", s, err)
		}
		if _, err := repo.Count(ctx, qq); err != nil {
			t.Fatalf("sort=%q count 失败: %v", s, err)
		}
	}
}

// testConf 造一个只带表前缀的配置。
//
// ⚠️ 必须显式传前缀：NewPurchaseFaceRepo 在前缀为空时会退回默认的 la_，
// 而本项目的真实前缀是 x_ —— 传 nil 会去查 xmarket_test.la_market_list_purchase，
// 报 "Table doesn't exist"。表前缀配错就是查错表，属于最难一眼看出的错误。
func testConf(prefix string) *conf.Data {
	return &conf.Data{
		Database: &conf.Data_Database{Prefix: prefix},
	}
}
