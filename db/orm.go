package db

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// column 描述结构体字段到数据库列的映射。
type column struct {
	name  string
	index []int
	auto  bool // 自增列，Insert 时跳过
	pk    bool // 主键，Update 时跳过
}

// Dialect 决定 SQL 占位符的写法。
type Dialect int

const (
	// Question 使用 ? 占位符（MySQL、SQLite）。
	Question Dialect = iota
	// Dollar 使用 $1、$2 占位符（PostgreSQL）。
	Dollar
)

// DialectFor 根据驱动名推断占位符方言。
func DialectFor(driver string) Dialect {
	switch strings.ToLower(driver) {
	case "postgres", "pgx", "pq":
		return Dollar
	default:
		return Question
	}
}

type crudConfig struct {
	dialect Dialect
}

// CRUDOption 定制 CRUD 辅助函数的行为。
type CRUDOption func(*crudConfig)

// WithDialect 指定占位符方言，默认 ?。
func WithDialect(dialect Dialect) CRUDOption {
	return func(c *crudConfig) { c.dialect = dialect }
}

func newCRUDConfig(opts []CRUDOption) *crudConfig {
	cfg := &crudConfig{dialect: Question}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (c *crudConfig) placeholder(index int) string {
	if c.dialect == Dollar {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

func validateIdentifier(name string) error {
	if !identifierPattern.MatchString(name) {
		return fmt.Errorf("db: invalid identifier %q", name)
	}
	return nil
}

// Insert 根据结构体生成并执行 INSERT。
//
// 标签约定：
//
//	db:"id,auto"  自增列，插入时跳过
//	db:"-"        不参与映射
//
// 返回结果的自增 ID 语义由驱动决定。
func Insert(ctx context.Context, exec Execer, table string, v any, opts ...CRUDOption) (sql.Result, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}

	cfg := newCRUDConfig(opts)
	structValue, err := structValueOf(v)
	if err != nil {
		return nil, err
	}

	columns := collectColumns(structValue.Type())
	names := make([]string, 0, len(columns))
	values := make([]any, 0, len(columns))

	for _, column := range columns {
		if column.auto {
			continue
		}
		field, ok := fieldByIndex(structValue, column.index)
		if !ok {
			continue
		}
		if err := validateIdentifier(column.name); err != nil {
			return nil, err
		}
		names = append(names, column.name)
		values = append(values, field.Interface())
	}

	if len(names) == 0 {
		return nil, fmt.Errorf("db: no insertable columns for %T", v)
	}

	placeholders := make([]string, len(names))
	for i := range names {
		placeholders[i] = cfg.placeholder(i + 1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table, strings.Join(names, ", "), strings.Join(placeholders, ", "))
	return exec.ExecContext(ctx, query, values...)
}

// Update 根据结构体生成并执行 UPDATE，跳过主键与自增列。
//
//	db:"id,pk"  主键列，更新时跳过
//
// where 子句由调用方提供，参数通过 args 传入，例如：
//
//	db.Update(ctx, d, "users", user, "id = ?", user.ID)
func Update(ctx context.Context, exec Execer, table string, v any, where string, args ...any) (sql.Result, error) {
	return UpdateWithOptions(ctx, exec, table, v, where, args, nil)
}

// UpdateWithOptions 与 Update 相同，但可以指定方言。
func UpdateWithOptions(ctx context.Context, exec Execer, table string, v any, where string, args []any, opts []CRUDOption) (sql.Result, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	where = strings.TrimSpace(where)
	if where == "" {
		return nil, fmt.Errorf("db: update requires a where clause")
	}

	cfg := newCRUDConfig(opts)
	structValue, err := structValueOf(v)
	if err != nil {
		return nil, err
	}

	assignments := make([]string, 0)
	values := make([]any, 0)
	index := 1

	for _, column := range collectColumns(structValue.Type()) {
		if column.pk || column.auto {
			continue
		}
		field, ok := fieldByIndex(structValue, column.index)
		if !ok {
			continue
		}
		if err := validateIdentifier(column.name); err != nil {
			return nil, err
		}
		assignments = append(assignments, fmt.Sprintf("%s = %s", column.name, cfg.placeholder(index)))
		values = append(values, field.Interface())
		index++
	}

	if len(assignments) == 0 {
		return nil, fmt.Errorf("db: no updatable columns for %T", v)
	}

	values = append(values, args...)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", table, strings.Join(assignments, ", "), where)
	return exec.ExecContext(ctx, query, values...)
}

// UpdateMap 按列的 map 生成并执行 UPDATE。
func UpdateMap(ctx context.Context, exec Execer, table string, data map[string]any, where string, args ...any) (sql.Result, error) {
	return UpdateMapWithOptions(ctx, exec, table, data, where, args, nil)
}

// UpdateMapWithOptions 与 UpdateMap 相同，但可以指定方言。
func UpdateMapWithOptions(ctx context.Context, exec Execer, table string, data map[string]any, where string, args []any, opts []CRUDOption) (sql.Result, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	where = strings.TrimSpace(where)
	if where == "" {
		return nil, fmt.Errorf("db: update requires a where clause")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("db: update requires at least one column")
	}

	names := make([]string, 0, len(data))
	for name := range data {
		if err := validateIdentifier(name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	// 保证生成语句稳定，便于排查问题。
	sortStrings(names)

	cfg := newCRUDConfig(opts)
	assignments := make([]string, 0, len(names))
	values := make([]any, 0, len(names)+len(args))
	for i, name := range names {
		assignments = append(assignments, fmt.Sprintf("%s = %s", name, cfg.placeholder(i+1)))
		values = append(values, data[name])
	}
	values = append(values, args...)

	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", table, strings.Join(assignments, ", "), where)
	return exec.ExecContext(ctx, query, values...)
}

// Delete 生成并执行 DELETE。where 为空会被拒绝，避免误删全表。
func Delete(ctx context.Context, exec Execer, table, where string, args ...any) (sql.Result, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	where = strings.TrimSpace(where)
	if where == "" {
		return nil, fmt.Errorf("db: delete requires a where clause")
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE %s", table, where)
	return exec.ExecContext(ctx, query, args...)
}

// Count 统计满足条件的行数。
func Count(ctx context.Context, queryer QueryRower, table, where string, args ...any) (int64, error) {
	if err := validateIdentifier(table); err != nil {
		return 0, err
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if where = strings.TrimSpace(where); where != "" {
		query += " WHERE " + where
	}

	var total int64
	if err := queryer.QueryRowContext(ctx, query, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// rowScanner 已由 QueryRower 提供，保留该注释以便阅读查询相关接口。

func structValueOf(v any) (reflect.Value, error) {
	value := reflect.ValueOf(v)
	if value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return reflect.Value{}, fmt.Errorf("db: nil struct pointer")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("db: expected struct, got %s", value.Kind())
	}
	return value, nil
}

func collectColumns(structType reflect.Type) []column {
	var out []column
	collect(typeAndIndex{structType, nil}, &out)
	return out
}

type typeAndIndex struct {
	structType reflect.Type
	index      []int
}

func collect(ti typeAndIndex, out *[]column) {
	for i := 0; i < ti.structType.NumField(); i++ {
		field := ti.structType.Field(i)
		index := make([]int, 0, len(ti.index)+1)
		index = append(index, ti.index...)
		index = append(index, i)

		if field.Anonymous {
			embedded := field.Type
			if embedded.Kind() == reflect.Ptr {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				collect(typeAndIndex{embedded, index}, out)
				continue
			}
		}

		if field.PkgPath != "" {
			continue
		}

		name, auto, pk, skip := parseFieldTag(field)
		if skip || name == "" {
			continue
		}
		*out = append(*out, column{name: name, index: index, auto: auto, pk: pk})
	}
}

func parseFieldTag(field reflect.StructField) (name string, auto, pk, skip bool) {
	tag := field.Tag.Get("db")
	if tag == "-" {
		return "", false, false, true
	}

	if tag != "" {
		parts := strings.Split(tag, ",")
		name = strings.TrimSpace(parts[0])
		for _, option := range parts[1:] {
			switch strings.TrimSpace(option) {
			case "auto":
				auto = true
			case "pk":
				pk = true
			}
		}
	}

	if name == "" {
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			n := strings.Split(jsonTag, ",")[0]
			if n == "-" {
				return "", false, false, true
			}
			name = n
		}
	}

	if name == "" {
		name = toSnake(field.Name)
	}
	return name, auto, pk, false
}

func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
