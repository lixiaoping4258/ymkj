package data

import (
	"os"

	"github.com/go-kratos/kratos/v2/log"
	"gopkg.in/yaml.v3"

	"github.com/lixiaoping4258/ymkj/internal/biz"
)

// projectConfig 对应原项目 config/project.php。
//
// ConfigService::get 的最后一道兜底是 `config('project.'.$type.'.'.$name)`。
// project.php 是一个纯 PHP 数组，Go 读不了，所以这里改成读同构的 project.yaml：
//
//	project:
//	  trade:
//	    outFee: 0.5
//	    buyOrSaleConfig: []
//
// 文件不存在也不会报错 —— 因为线上如果 la_config 表里都有记录，
// 这条兜底分支根本走不到。
type projectConfig struct {
	data map[string]any
}

// NewProjectConfig 路径通过环境变量 PROJECT_CONFIG 指定，
// 默认 configs/project.yaml。用环境变量而不是参数，是为了让 wire 能自动装配
// （多一个 string 参数 wire 就不知道注入什么了）。
func NewProjectConfig(logger log.Logger) biz.ProjectConfig {
	path := os.Getenv("PROJECT_CONFIG")
	if path == "" {
		path = "configs/project.yaml"
	}
	pc := &projectConfig{data: map[string]any{}}
	b, err := os.ReadFile(path)
	if err != nil {
		// 不是错误：没这个文件说明不需要兜底
		log.NewHelper(logger).Infof("未加载 project 配置(%s)，兜底分支将返回 null", path)
		return pc
	}
	var root struct {
		Project map[string]any `yaml:"project"`
	}
	if err := yaml.Unmarshal(b, &root); err != nil {
		log.NewHelper(logger).Errorf("解析 project 配置失败 %s: %v", path, err)
		return pc
	}
	// yaml v3 解出来是 map[string]interface{}，但嵌套层可能是 map[string]interface{}
	if root.Project != nil {
		pc.data = root.Project
	}
	log.NewHelper(logger).Infof("已加载 project 配置，共 %d 个分组", len(pc.data))
	return pc
}

// Lookup 等价于 config('project.{type}.{name}')。
func (p *projectConfig) Lookup(typ, name string) (any, bool) {
	group, ok := p.data[typ]
	if !ok {
		return nil, false
	}
	m, ok := group.(map[string]any)
	if !ok {
		return nil, false
	}
	v, ok := m[name]
	return v, ok
}
