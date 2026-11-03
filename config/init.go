package config

// MustLoad 加载 JSON 配置文件，失败时 panic，适合在 main 中完成初始化。
//
//	cfg := config.MustLoad("config.json")
//	addr := cfg.String("http.addr", ":8080")
func MustLoad(path string, opts ...Option) *Config {
	cfg, err := Load(append([]Option{WithFile(path)}, opts...)...)
	if err != nil {
		panic(err)
	}
	return cfg
}

// MustNew 不加载文件，只应用选项（例如默认值与 env 前缀），失败时 panic。
func MustNew(opts ...Option) *Config {
	cfg, err := Load(opts...)
	if err != nil {
		panic(err)
	}
	return cfg
}
