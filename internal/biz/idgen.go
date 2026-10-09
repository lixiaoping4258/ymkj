package biz

import "context"

// IDGenerator 对应原项目 extend/xlu/Id.php 的 gen()。
//
// ⚠️⚠️ 这**不是纯函数**，而是一个**跨系统共享状态**的分布式 ID 生成器。
// Go 侧必须与 PHP **共用同一把 Redis 键**，否则两套系统会各自生成 ID，
// 导致**同一个 ID 被两边各用一次**（数据损坏级问题，不是"ID 重复报错"那么轻）。
//
// 原实现（extend/xlu/Id.php:32）：
//
//	$redisCache = Cache::store('redis');
//	$key = 'id:gen';  $prefixKey = 'id:prefix';      // 经 TP 缓存前缀 -> la:id:gen / la:id:prefix
//	if (!$redisCache->has($key)) { $redisCache->delete($prefixKey); }
//	$prefix = $redisCache->get($prefixKey);
//	if (!$prefix) { $prefix = time() + $offset; $redisCache->set($prefixKey, $prefix); }
//	$suffix = $redisCache->inc($key);
//	if ($suffix >= 99995) { $redisCache->set($key, 0); $redisCache->set($prefixKey, time() + $offset); }
//	return intval(sprintf('%s%05d', $prefix, $suffix));
//
// **实测确认（2026-10-09）**：这两个键在 Redis 里是**裸整数字符串，没有被 TP 序列化**
//
//	la:id:gen     -> "230"           （对比 la:order:gen 是 a:2:{...} 序列化数组）
//	la:id:prefix  -> "1786083621"
//
// 所以 Go 可以直接 GET/INCR/SET 它们，不需要复刻 TP 的序列化格式。
//
// 生成结果形如 `<秒级时间戳><5 位零填充序号>`：1786083621 + 230 -> 178608362100230。
// 与库里 ID 形态一致（如 178609302407853）。
type IDGenerator interface {
	// Gen 对应 Id::gen($offset)。offset 一般为 0。
	Gen(ctx context.Context, offset int64) (int64, error)
}

// IDGeneratorOffsets 原实现里 offset 的默认值。
const IDGeneratorDefaultOffset = 0
