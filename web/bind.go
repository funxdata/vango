package web

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// bindQuery 把 url.Values 按结构体 tag 绑定到 target。
//
// 取值顺序：query > json > 字段名；`json:"-"` 表示忽略该字段。
// 支持基本类型、指针、字符串切片以及实现了 encoding.TextUnmarshaler 的类型（如 time.Time）。
func bindQuery(target any, values url.Values) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return badRequest("bind target must be a non-nil pointer")
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return badRequest("bind target must point to a struct")
	}
	return bindStruct(elem, values)
}

func bindStruct(structValue reflect.Value, values url.Values) error {
	structType := structValue.Type()

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)

		if field.Anonymous && fieldValue.Kind() == reflect.Struct {
			if err := bindStruct(fieldValue, values); err != nil {
				return err
			}
			continue
		}
		if field.PkgPath != "" || !fieldValue.CanSet() {
			continue
		}

		name := bindName(field)
		if name == "" || name == "-" {
			continue
		}

		raw, ok := values[name]
		if !ok || len(raw) == 0 {
			continue
		}
		if err := assignValue(fieldValue, raw, name); err != nil {
			return err
		}
	}

	return nil
}

func bindName(field reflect.StructField) string {
	for _, tag := range []string{"query", "json"} {
		value, ok := field.Tag.Lookup(tag)
		if !ok {
			continue
		}
		if name := strings.Split(value, ",")[0]; name != "" {
			return name
		}
	}
	return field.Name
}

func assignValue(field reflect.Value, raw []string, name string) error {
	if field.CanAddr() {
		if unmarshaler, ok := field.Addr().Interface().(encoding.TextUnmarshaler); ok {
			if err := unmarshaler.UnmarshalText([]byte(raw[0])); err != nil {
				return badRequest(fmt.Sprintf("field %s: %v", name, err))
			}
			return nil
		}
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw[0])

	case reflect.Bool:
		value, err := strconv.ParseBool(raw[0])
		if err != nil {
			return badRequest(fmt.Sprintf("field %s: %v", name, err))
		}
		field.SetBool(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value, err := strconv.ParseInt(raw[0], 10, 64)
		if err != nil {
			return badRequest(fmt.Sprintf("field %s: %v", name, err))
		}
		field.SetInt(value)

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value, err := strconv.ParseUint(raw[0], 10, 64)
		if err != nil {
			return badRequest(fmt.Sprintf("field %s: %v", name, err))
		}
		field.SetUint(value)

	case reflect.Float32, reflect.Float64:
		value, err := strconv.ParseFloat(raw[0], 64)
		if err != nil {
			return badRequest(fmt.Sprintf("field %s: %v", name, err))
		}
		field.SetFloat(value)

	case reflect.Ptr:
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		return assignValue(field.Elem(), raw, name)

	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			field.Set(reflect.ValueOf(raw).Convert(field.Type()))
			return nil
		}
		slice := reflect.MakeSlice(field.Type(), 0, len(raw))
		for _, item := range raw {
			elem := reflect.New(field.Type().Elem()).Elem()
			if err := assignValue(elem, []string{item}, name); err != nil {
				return err
			}
			slice = reflect.Append(slice, elem)
		}
		field.Set(slice)

	default:
		return badRequest(fmt.Sprintf("field %s: unsupported type %s", name, field.Type()))
	}

	return nil
}
