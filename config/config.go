// Package config 提供零依赖、线程安全的配置读取能力。
//
// 设计目标：
//   - 默认值 → 配置文件（JSON）→ 环境变量，后者覆盖前者；
//   - 用点号路径访问嵌套配置，例如 cfg.String("http.addr", ":8080")；
//   - 环境变量名由前缀 + 路径组成，点号替换为下划线并大写，
//     例如 VANGO_HTTP_ADDR 覆盖 http.addr。
//
// 需要 YAML/TOML 时，可自行解析出 map[string]any 后调用 Merge。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultEnvPrefix 是环境变量前缀的默认值。
const DefaultEnvPrefix = "VANGO"

type store struct {
	mu     sync.RWMutex
	values map[string]any
}

// Config 是配置集合。多个 Sub 视图共享同一份底层数据。
type Config struct {
	st        *store
	envPrefix string
	prefix    string
	file      string
}

// Option 用于定制 Config 的构建。
type Option func(*Config)

// WithFile 指定需要加载的 JSON 配置文件路径。
func WithFile(path string) Option {
	return func(c *Config) { c.file = path }
}

// WithEnvPrefix 指定环境变量前缀，传空字符串表示禁用环境变量覆盖。
func WithEnvPrefix(prefix string) Option {
	return func(c *Config) { c.envPrefix = prefix }
}

// WithValues 注入初始配置，常用于提供默认值。
func WithValues(values map[string]any) Option {
	return func(c *Config) { c.Merge(values) }
}

// New 创建一个空的 Config。
func New(opts ...Option) *Config {
	c := &Config{
		st:        &store{values: make(map[string]any)},
		envPrefix: DefaultEnvPrefix,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Load 创建 Config，并在指定了文件时立即加载。
func Load(opts ...Option) (*Config, error) {
	c := New(opts...)
	if c.file != "" {
		if err := c.LoadFile(c.file); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// LoadFile 读取并合并一个 JSON 配置文件。
func (c *Config) LoadFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: read %s: %w", path, err)
	}

	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	c.file = path
	c.Merge(values)
	return nil
}

// File 返回已加载的配置文件路径，未加载时为空。
func (c *Config) File() string { return c.file }

// Merge 深度合并配置，嵌套的 map 会按点号路径展开。
func (c *Config) Merge(values map[string]any) {
	flatten("", values, func(path string, value any) {
		c.Set(path, value)
	})
}

// Set 按点号路径写入一个配置项。
func (c *Config) Set(key string, value any) {
	full := c.fullKey(key)
	if full == "" {
		return
	}

	c.st.mu.Lock()
	defer c.st.mu.Unlock()
	setPath(c.st.values, full, value)
}

// Get 读取配置项，先查环境变量再查内存值。
func (c *Config) Get(key string) (any, bool) {
	return c.lookup(key)
}

// Has 判断配置项是否存在。
func (c *Config) Has(key string) bool {
	_, ok := c.lookup(key)
	return ok
}

// Sub 返回带前缀的子配置视图，envPrefix 与已加载的配置文件会一并继承。
func (c *Config) Sub(prefix string) *Config {
	return &Config{
		st:        c.st,
		envPrefix: c.envPrefix,
		prefix:    c.fullKey(prefix),
		file:      c.file,
	}
}

// String 读取字符串配置，缺失时返回 def。
func (c *Config) String(key, def string) string {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	return toString(v, def)
}

// MustString 读取字符串配置，缺失或为空时 panic，适合启动期的必填项。
func (c *Config) MustString(key string) string {
	v, ok := c.lookup(key)
	if !ok {
		panic(fmt.Sprintf("config: required key %q is missing", c.fullKey(key)))
	}
	s := toString(v, "")
	if s == "" {
		panic(fmt.Sprintf("config: required key %q is empty", c.fullKey(key)))
	}
	return s
}

// Int 读取整数配置，缺失或无法解析时返回 def。
func (c *Config) Int(key string, def int) int {
	return int(c.Int64(key, int64(def)))
}

// Int64 读取 int64 配置。
func (c *Config) Int64(key string, def int64) int64 {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	return toInt64(v, def)
}

// Float64 读取浮点配置。
func (c *Config) Float64(key string, def float64) float64 {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return def
		}
		return f
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return def
		}
		return f
	default:
		return def
	}
}

// Bool 读取布尔配置。
func (c *Config) Bool(key string, def bool) bool {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "1", "t", "true", "yes", "y", "on":
			return true
		case "0", "f", "false", "no", "n", "off":
			return false
		default:
			return def
		}
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	default:
		return def
	}
}

// Duration 读取时长配置。
//
// 字符串支持 time.ParseDuration 的写法（如 "30s"、"2m"）；
// 数字按秒计算；缺失或无法解析时返回 def。
func (c *Config) Duration(key string, def time.Duration) time.Duration {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	switch t := v.(type) {
	case time.Duration:
		return t
	case int:
		return time.Duration(t) * time.Second
	case int64:
		return time.Duration(t) * time.Second
	case float64:
		return time.Duration(t * float64(time.Second))
	case string:
		s := strings.TrimSpace(t)
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Duration(n) * time.Second
		}
		return def
	default:
		return def
	}
}

// Strings 读取字符串列表，字符串形式的配置按逗号切分。
func (c *Config) Strings(key string, def []string) []string {
	v, ok := c.lookup(key)
	if !ok {
		return def
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, toString(item, ""))
		}
		return out
	case string:
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return def
	}
}

// Keys 返回所有配置项的完整点号路径，按字典序排列。
func (c *Config) Keys() []string {
	c.st.mu.RLock()
	defer c.st.mu.RUnlock()

	keys := make([]string, 0, len(c.st.values))
	var walk func(prefix string, values map[string]any)
	walk = func(prefix string, values map[string]any) {
		for k, v := range values {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			if m, ok := v.(map[string]any); ok {
				walk(key, m)
				continue
			}
			keys = append(keys, key)
		}
	}
	walk("", c.st.values)
	sort.Strings(keys)
	return keys
}

// Unmarshal 把配置解码进结构体，字段可用 json tag 指定键名。
// 注意：子视图的 prefix 不参与解码。
func (c *Config) Unmarshal(v any) error {
	c.st.mu.RLock()
	raw, err := json.Marshal(c.st.values)
	c.st.mu.RUnlock()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

func (c *Config) lookup(key string) (any, bool) {
	full := c.fullKey(key)
	if full == "" {
		return nil, false
	}

	if envKey := c.envKey(full); envKey != "" {
		if value, ok := os.LookupEnv(envKey); ok {
			return value, true
		}
	}

	c.st.mu.RLock()
	defer c.st.mu.RUnlock()
	return lookupPath(c.st.values, full)
}

// lookupPath 按点号路径逐层查找嵌套配置。
func lookupPath(values map[string]any, path string) (any, bool) {
	var current any = values
	for _, part := range strings.Split(path, ".") {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func (c *Config) fullKey(key string) string {
	key = strings.Trim(key, ".")
	if c.prefix == "" {
		return key
	}
	if key == "" {
		return c.prefix
	}
	return c.prefix + "." + key
}

func (c *Config) envKey(full string) string {
	if c.envPrefix == "" || full == "" {
		return ""
	}
	replacer := strings.NewReplacer(".", "_", "-", "_")
	return strings.ToUpper(c.envPrefix + "_" + replacer.Replace(full))
}

func setPath(values map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := values
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = value
}

func flatten(prefix string, values map[string]any, fn func(path string, value any)) {
	for k, v := range values {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if m, ok := v.(map[string]any); ok {
			flatten(key, m, fn)
			continue
		}
		fn(key, v)
	}
}

func toString(v any, def string) string {
	switch t := v.(type) {
	case nil:
		return def
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(v)
	}
}

func toInt64(v any, def int64) int64 {
	switch t := v.(type) {
	case int:
		return int64(t)
	case int8:
		return int64(t)
	case int16:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint:
		return int64(t)
	case uint8:
		return int64(t)
	case uint16:
		return int64(t)
	case uint32:
		return int64(t)
	case uint64:
		return int64(t)
	case float32:
		return int64(t)
	case float64:
		return int64(t)
	case bool:
		if t {
			return 1
		}
		return 0
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return def
		}
		return n
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return def
		}
		return n
	default:
		return def
	}
}
