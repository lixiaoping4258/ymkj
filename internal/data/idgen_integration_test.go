package data

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"gopkg.in/yaml.v3"
)

// 集成测试：验证 Go 的 ID 生成器与 PHP 共用同一把 Redis 键、且生成规则一致。
//
// ⚠️ 这个测试会**真的消耗共享 ID**（INCR 一次）—— 这正是要验证的行为：
// PHP 与 Go 交替调用时必须落在同一条序列上、不重复。
// 每次调用只消耗 1 个 ID，对一个测试库是安全的。
//
// 期望值的判定方式（不是硬编码数字，因为计数器会变）：
//
//	调之前读 la:id:prefix = P、la:id:gen = N
//	调之后应得到 P 与 N+1 按 "%s%05d" 拼接的值
//
// 同时断言它**真的动了共享键**（N 变成 N+1）—— 如果 Go 用了隔离前缀，
// 这条会失败，这正是要防的。
func TestIDGenerator_SharesRedisKeysWithPHP(t *testing.T) {
	cfgPath := findRepoRoot(t) + "/configs/config.local.yaml"
	if _, err := os.Stat(cfgPath); err != nil {
		t.Skip("configs/config.local.yaml 不存在，跳过集成测试")
	}
	var cfg struct {
		Data struct {
			Redis struct {
				Addr     string `yaml:"addr"`
				Password string `yaml:"password"`
				DB       int    `yaml:"db"`
			} `yaml:"redis"`
		} `yaml:"data"`
	}
	raw, _ := os.ReadFile(cfgPath)
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("解析配置失败: %v", err)
	}
	cli := redis.NewClient(&redis.Options{
		Addr: cfg.Data.Redis.Addr, Password: cfg.Data.Redis.Password, DB: cfg.Data.Redis.DB,
	})
	ctx := context.Background()
	if err := cli.Ping(ctx).Err(); err != nil {
		t.Skipf("连不上 Redis，跳过: %v", err)
	}

	// 调之前的状态
	beforePrefix, errP := cli.Get(ctx, idPrefixKey).Int64()
	beforeGen, errG := cli.Get(ctx, idGenKey).Int64()
	if errP != nil || errG != nil {
		t.Skipf("共享键尚不存在（%v / %v），先让 PHP 调一次 Id::gen 再跑", errP, errG)
	}
	t.Logf("调用前: la:id:prefix=%d  la:id:gen=%d", beforePrefix, beforeGen)

	gen := NewIDGenerator(cli)
	got, err := gen.Gen(ctx, 0)
	if err != nil {
		t.Fatalf("Gen 失败: %v", err)
	}

	// 调之后
	afterPrefix, _ := cli.Get(ctx, idPrefixKey).Int64()
	afterGen, _ := cli.Get(ctx, idGenKey).Int64()
	t.Logf("调用后: la:id:prefix=%d  la:id:gen=%d  Go 返回=%d", afterPrefix, afterGen, got)

	// ① 必须真的推进了共享计数器（防"用了隔离前缀"）
	if afterGen != beforeGen+1 {
		t.Fatalf("共享键 la:id:gen 应 +1（%d -> %d），实际 %d —— 说明没共用键或没走 INCR",
			beforeGen, beforeGen+1, afterGen)
	}
	// ② 前缀必须没变（除非刚触发了 99995 重置）
	if afterPrefix == beforePrefix {
		want := fmt.Sprintf("%d%05d", beforePrefix, beforeGen+1)
		if fmt.Sprintf("%d", got) != want {
			t.Fatalf("ID 不符：得到 %d，按 sprintf('%%s%%05d') 应为 %s", got, want)
		}
	} else {
		t.Logf("前缀变了（%d -> %d），可能刚触发 %d 重置，只校验形态", beforePrefix, afterPrefix, idGenMaxSuffix)
	}

	// ③ 形态：必须等于 <prefix> + 5 位序号 的拼接
	wantLen := len(fmt.Sprintf("%d", afterPrefix)) + 5
	if len(fmt.Sprintf("%d", got)) != wantLen && got < 100000 {
		t.Errorf("ID 位数 %d 与 <prefix>+5 位（%d）不符", len(fmt.Sprintf("%d", got)), wantLen)
	}

	// ④ 连续两次必须严格递增且不重复（与 PHP 交替调用时同一保证）
	second, err := gen.Gen(ctx, 0)
	if err != nil {
		t.Fatalf("第二次 Gen 失败: %v", err)
	}
	if second <= got {
		t.Fatalf("连续生成应递增：%d 之后得到 %d", got, second)
	}
}
