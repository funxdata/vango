package db

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// ScanAll 把查询结果扫描成结构体切片。
//
// 列名通过 `db` tag 匹配，其次 `json` tag，最后使用字段名的 snake_case。
// 结果集中多余的列会被忽略；结构体中未出现在结果集里的字段保持零值。
//
//	rows, err := d.QueryContext(ctx, "SELECT id, name FROM users")
//	users, err := db.ScanAll[User](rows)
func ScanAll[T any](rows *sql.Rows) ([]T, error) {
	if rows == nil {
		return nil, fmt.Errorf("db: ScanAll requires non-nil rows")
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("db: read columns: %w", err)
	}

	var zero T
	structType := reflect.TypeOf(zero)
	if structType == nil {
		return nil, fmt.Errorf("db: ScanAll requires a concrete struct type")
	}
	if structType.Kind() == reflect.Ptr {
		structType = structType.Elem()
	}
	if structType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("db: ScanAll requires a struct type, got %s", structType)
	}

	byName := make(map[string][]int, len(columns))
	for _, column := range collectColumns(structType) {
		byName[column.name] = column.index
	}

	indexes := make([][]int, len(columns))
	for i, column := range columns {
		indexes[i] = byName[column]
	}

	var out []T
	for rows.Next() {
		var item T
		value := reflect.ValueOf(&item).Elem()

		dest := make([]any, len(columns))
		var discard []any
		for i := range columns {
			if indexes[i] == nil {
				discard = append(discard, new(any))
				dest[i] = discard[len(discard)-1]
				continue
			}
			field, ok := fieldByIndex(value, indexes[i])
			if !ok || !field.CanAddr() {
				dest[i] = new(any)
				continue
			}
			dest[i] = field.Addr().Interface()
		}

		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("db: scan row: %w", err)
		}
		out = append(out, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: iterate rows: %w", err)
	}
	return out, nil
}

// ScanOne 把单行查询结果扫描成一个结构体，没有数据时返回 (nil, nil)。
func ScanOne[T any](rows *sql.Rows) (*T, error) {
	items, err := ScanAll[T](rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func fieldByIndex(value reflect.Value, index []int) (reflect.Value, bool) {
	for _, i := range index {
		if value.Kind() == reflect.Ptr {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct || i >= value.NumField() {
			return reflect.Value{}, false
		}
		value = value.Field(i)
	}
	return value, true
}

// toSnake 把 CamelCase 转成 snake_case。
func toSnake(name string) string {
	var builder strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				builder.WriteByte('_')
			}
			builder.WriteRune(r - 'A' + 'a')
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}
