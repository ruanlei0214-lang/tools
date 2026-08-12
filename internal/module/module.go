// Package module 定义工具模块的接入契约。
package module

import "context"

// Module 是一个自包含的功能单元。模块之间不互相引用，
// 唯一的接线点是 internal/modules.All()。
type Module interface {
	// ID 在模块间唯一，用于启动查重与日志定位。
	ID() string
	// Bindings 返回暴露给前端的对象，Wails 据此生成 TypeScript 绑定。
	Bindings() []any
}

// Startupper 由需要 Wails 运行时上下文的模块实现，可选。
type Startupper interface {
	Startup(ctx context.Context)
}
