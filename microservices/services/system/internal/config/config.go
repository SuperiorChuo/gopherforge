// Package config 提供 12-factor、仅环境变量驱动的配置，供
// system 服务使用。它刻意保持与单体 config 包相同的结构体形态、全局 Cfg
// 变量、辅助方法与环境变量名称，使从单体拷贝的代码无需改动即可继续
// 工作，并让 docker-compose 环境保持一致。
package config

var Cfg Config

// Load 用叠加在 Defaults 之上的环境变量填充包级 Cfg。
// 环境变量名称与单体完全一致。
func Load() error {
	cfg := Defaults()
	applyEnv(&cfg)
	if err := validate(cfg); err != nil {
		return err
	}
	Cfg = cfg
	return nil
}
