// Package hello 是模块框架的最小示例。
package hello

import "fmt"

type Module struct {
	svc *Service
}

func New() *Module { return &Module{svc: &Service{}} }

func (m *Module) ID() string { return "hello" }

func (m *Module) Bindings() []any { return []any{m.svc} }

// Service 上所有导出方法都会自动暴露给前端。
type Service struct{}

func (s *Service) Greet(name string) string {
	if name == "" {
		name = "世界"
	}
	return fmt.Sprintf("你好，%s！这句话来自 Go。", name)
}
