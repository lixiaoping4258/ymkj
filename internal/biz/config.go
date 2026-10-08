package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// ConfigRepo 是配置表的读取接口。
//
// 对应原项目 app/common/model/Config.php + ConfigService 里的 DB 查询部分。
// 按 Kratos 约定，接口定义在 biz（消费方），实现放 data。
type ConfigRepo interface {
	// GetValue 取单条配置的原始 value。
	// 返回 (nil, nil) 表示记录不存在 —— 对应 PHP findOrEmpty() 后取不到值。
	// cacheable 为 true 时才走缓存（复刻 PHP 对空 name 的分支不查缓存的行为）。
	GetValue(ctx context.Context, typ, name string) (any, bool, error)

	// GetAll 取某个 type 下所有 name => value。
	GetAll(ctx context.Context, typ string) (map[string]any, error)
}

// ProjectConfig 对应原项目 config/project.php。
//
// ConfigService::get 在「DB 查不到 + 没给默认值」时会回退到这里：
//
//	return config('project.' . $type . '.' . $name);
//
// 这个文件是纯 PHP 数组，迁移时要么手工转成 project.yaml，
// 要么确认线上这些 key 都已在 la_config 表里有记录（那就走不到这一步）。
type ProjectConfig interface {
	Lookup(typ, name string) (any, bool)
}

// ErrNotConfigured 表示既没查到配置、也没默认值、project 配置里也没有。
// 原项目这种情况下会返回 null，前端拿到 null。
var ErrNotConfigured = errNotConfigured{}

type errNotConfigured struct{}

func (errNotConfigured) Error() string { return "config not found" }

// ConfigUsecase 复刻 app/common/service/ConfigService.php。
type ConfigUsecase struct {
	repo    ConfigRepo
	project ProjectConfig
	log     *log.Helper
}

func NewConfigUsecase(repo ConfigRepo, project ProjectConfig, logger log.Logger) *ConfigUsecase {
	return &ConfigUsecase{
		repo:    repo,
		project: project,
		log:     log.NewHelper(logger),
	}
}

// Get 等价于 ConfigService::get($type, $name)。
// 即 PHP 的 $default_value === null（未传默认值）。
func (uc *ConfigUsecase) Get(ctx context.Context, typ, name string) (any, error) {
	return uc.get(ctx, typ, name, nil, false)
}

// GetDefault 等价于 ConfigService::get($type, $name, $default)。
func (uc *ConfigUsecase) GetDefault(ctx context.Context, typ, name string, def any) (any, error) {
	return uc.get(ctx, typ, name, def, true)
}

// GetGroup 等价于 ConfigService::get($type)，即取整个 type 下所有配置。
func (uc *ConfigUsecase) GetGroup(ctx context.Context, typ string) (map[string]any, error) {
	raw, err := uc.repo.GetAll(ctx, typ)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = DecodeConfigValue(v)
	}
	if len(out) == 0 {
		return nil, nil // PHP 里 $data 为空数组时函数没有 return，即返回 null
	}
	return out, nil
}

// get 是 ConfigService::get 的逐行对照实现。
//
// 注意 PHP 里 $value 的流转顺序，任何一步顺序变了行为都会变：
//
//	缓存 -> DB -> json_decode -> truthy? -> zero-like? -> default? -> project 配置
func (uc *ConfigUsecase) get(ctx context.Context, typ, name string, def any, defSet bool) (any, error) {
	// PHP: if (!empty($name)) —— name 为空串或 "0" 时走下面「取整组」的分支
	if name == "" || name == "0" {
		return uc.GetGroup(ctx, typ)
	}

	raw, found, err := uc.repo.GetValue(ctx, typ, name)
	if err != nil {
		return nil, err
	}

	var value any
	if found {
		value = DecodeConfigValue(raw)
	} else {
		// 记录不存在：PHP 的 $value 是 null，跳过 json_decode
		value = nil
	}

	if PhpTruthy(value) {
		return value, nil
	}
	// PHP: 返回特殊值 0 '0'
	if PhpIsZeroLike(value) {
		return value, nil
	}
	if defSet {
		return def, nil
	}
	// PHP: return config('project.' . $type . '.' . $name)
	if uc.project != nil {
		if v, ok := uc.project.Lookup(typ, name); ok {
			return v, nil
		}
	}
	uc.log.WithContext(ctx).Debugf("config 未命中: type=%s name=%s", typ, name)
	return nil, nil // PHP 该分支返回 null
}
