// Package toolbox 是工具箱级的绑定，不属于任何一个功能模块。
// 顶栏改地址写 toolbox-config.json，这条配置三个模块共用，放在模块里会让
// 没编进产物的模块丢了入口。
package toolbox

import "embedtools/internal/module"

type Service struct{}

func New() *Service { return &Service{} }

// SaveHost 把顶栏改过的地址写进 exe 同目录的 toolbox-config.json。
// 只动 host，凭据不动。
func (s *Service) SaveHost(host string) error {
	return module.SaveHost(host)
}
