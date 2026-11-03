package vango

// Module 是可插拔的应用模块。
//
//	app.Register(user.Module, order.Module)
type Module interface {
	// Name 返回模块名，用于日志与排查。
	Name() string
	// Register 完成模块初始化，通常在这里注册路由、启动钩子与关闭钩子。
	Register(app *App) error
}

// ModuleFunc 把函数适配成 Module。
type ModuleFunc struct {
	ModuleName string
	Fn         func(*App) error
}

// Name 实现 Module。
func (m ModuleFunc) Name() string { return m.ModuleName }

// Register 实现 Module。
func (m ModuleFunc) Register(app *App) error {
	if m.Fn == nil {
		return nil
	}
	return m.Fn(app)
}

// NewModule 用一个函数创建模块。
func NewModule(name string, fn func(*App) error) ModuleFunc {
	return ModuleFunc{ModuleName: name, Fn: fn}
}
