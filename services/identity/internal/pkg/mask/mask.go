package mask

import (
	"net/netip"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

type fieldMetadata struct {
	index    int
	maskType string
}

var structFieldCache sync.Map

// MaskValue applies a masking strategy to a string value.
func MaskValue(maskType string, input string) string {
	if input == "" {
		return ""
	}

	switch strings.ToLower(maskType) {
	case "email":
		return maskEmail(input)
	case "phone":
		return maskPhone(input)
	case "ip":
		return maskIP(input)
	case "token", "hash":
		return maskTokenLike(input)
	case "path":
		return maskPath(input)
	case "full":
		return "***"
	default:
		return input
	}
}

// CloneAndMask clones structured data and applies tag-based masking.
func CloneAndMask[T any](data T, shouldMask bool) T {
	if !shouldMask {
		return data
	}

	value := reflect.ValueOf(data)
	if !value.IsValid() {
		var zero T
		return zero
	}

	cloned := cloneValue(value)
	applyMask(cloned)

	result, ok := cloned.Interface().(T)
	if ok {
		return result
	}
	return data
}

// CloneAndMaskAny clones structured data behind an any value and applies tag-based masking.
func CloneAndMaskAny(data any, shouldMask bool) any {
	if !shouldMask || data == nil {
		return data
	}

	value := reflect.ValueOf(data)
	if !value.IsValid() {
		return data
	}

	cloned := cloneValue(value)
	applyMask(cloned)
	return cloned.Interface()
}

func cloneValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneValue(value.Elem()))
		return cloned
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneValue(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	case reflect.Struct:
		if isScalarStruct(value.Type()) {
			return value
		}

		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			field := cloned.Field(i)
			if !field.CanSet() {
				continue
			}
			field.Set(cloneValue(value.Field(i)))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneValue(value.Index(i)))
		}
		return cloned
	case reflect.Array:
		cloned := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			cloned.Index(i).Set(cloneValue(value.Index(i)))
		}
		return cloned
	default:
		return value
	}
}

func applyMask(value reflect.Value) {
	if !value.IsValid() {
		return
	}

	switch value.Kind() {
	case reflect.Pointer:
		if !value.IsNil() {
			applyMask(value.Elem())
		}
	case reflect.Interface:
		if !value.IsNil() {
			applyMask(value.Elem())
		}
	case reflect.Struct:
		if isScalarStruct(value.Type()) {
			return
		}
		for _, metadata := range cachedFieldMetadata(value.Type()) {
			field := value.Field(metadata.index)
			if !field.CanSet() {
				continue
			}
			if metadata.maskType != "" && field.Kind() == reflect.String && field.CanSet() {
				field.SetString(MaskValue(metadata.maskType, field.String()))
				continue
			}
			applyMask(field)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			applyMask(value.Index(i))
		}
	}
}

func cachedFieldMetadata(t reflect.Type) []fieldMetadata {
	if cached, ok := structFieldCache.Load(t); ok {
		return cached.([]fieldMetadata)
	}

	metadata := make([]fieldMetadata, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		metadata = append(metadata, fieldMetadata{
			index:    i,
			maskType: field.Tag.Get("mask"),
		})
	}
	structFieldCache.Store(t, metadata)
	return metadata
}

func isScalarStruct(t reflect.Type) bool {
	return t.PkgPath() == "time" && t.Name() == "Time"
}

func maskEmail(input string) string {
	parts := strings.SplitN(input, "@", 2)
	if len(parts) != 2 {
		return input
	}
	local := parts[0]
	if len(local) <= 1 {
		return "***@" + parts[1]
	}
	return local[:1] + "***" + local[len(local)-1:] + "@" + parts[1]
}

func maskPhone(input string) string {
	if len(input) < 11 {
		return input
	}
	return input[:3] + "****" + input[len(input)-4:]
}

func maskIP(input string) string {
	addr, err := netip.ParseAddr(input)
	if err != nil {
		return input
	}

	if addr.Is4() {
		bytes := addr.As4()
		return strings.Join([]string{
			intToString(bytes[0]),
			intToString(bytes[1]),
			"*",
			"*",
		}, ".")
	}

	bytes := addr.As16()
	first := uint16(bytes[0])<<8 | uint16(bytes[1])
	second := uint16(bytes[2])<<8 | uint16(bytes[3])
	return strings.ToLower(strings.Join([]string{
		hexUint16(first),
		hexUint16(second),
		"*",
		"*",
		"*",
		"*",
		"*",
		"*",
	}, ":"))
}

func maskTokenLike(input string) string {
	if len(input) < 8 {
		return "***"
	}
	return input[:4] + "***" + input[len(input)-4:]
}

func maskPath(input string) string {
	base := filepath.Base(input)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "***"
	}
	return "***/" + base
}

func intToString(v byte) string {
	return strconv.Itoa(int(v))
}

func hexUint16(v uint16) string {
	return strconv.FormatUint(uint64(v), 16)
}
