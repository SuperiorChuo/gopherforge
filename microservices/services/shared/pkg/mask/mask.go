// Package mask provides sensitive-field masking (struct tag driven).

package mask

import (
	"encoding/json"
	"net/netip"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

const (
	// RedactedValue is the stable placeholder returned for secret values.
	RedactedValue = "********"
	// InvalidJSONRedactedValue replaces bodies that cannot be parsed safely.
	InvalidJSONRedactedValue = "[request body redacted: invalid or truncated JSON]"
)

var sensitiveFieldNames = map[string]struct{}{
	"password":          {},
	"password_hash":     {},
	"old_password":      {},
	"new_password":      {},
	"current_password":  {},
	"recovery_code":     {},
	"recovery_codes":    {},
	"backup_code":       {},
	"backup_codes":      {},
	"token":             {},
	"access_token":      {},
	"refresh_token":     {},
	"id_token":          {},
	"secret":            {},
	"api_key":           {},
	"amap_key":          {},
	"secret_key":        {},
	"access_key_secret": {},
	"client_secret":     {},
	"private_key":       {},
	"signing_key":       {},
	"authorization":     {},
}

var nonSensitiveFieldNames = map[string]struct{}{
	"must_change_password": {},
}

// IsSensitiveField reports whether a JSON field conventionally contains a secret.
func IsSensitiveField(field string) bool {
	normalized := normalizeFieldName(field)
	if _, ok := nonSensitiveFieldNames[normalized]; ok {
		return false
	}
	if _, ok := sensitiveFieldNames[normalized]; ok {
		return true
	}
	for _, suffix := range []string{
		"_password",
		"_password_hash",
		"_recovery_code",
		"_recovery_codes",
		"_backup_code",
		"_backup_codes",
		"_token",
		"_secret",
		"_api_key",
		"_private_key",
		"_signing_key",
	} {
		if strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return false
}

// IsRedactedValue reports whether a client sent back one of the supported
// display-only placeholders instead of a new secret.
func IsRedactedValue(value any) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(text) {
	case "***", RedactedValue, "[REDACTED]":
		return true
	default:
		return false
	}
}

// RedactJSON masks nested secret fields. Invalid or truncated JSON is replaced
// wholesale because partially parsed data cannot be proven secret-free.
func RedactJSON(body string) string {
	if body == "" {
		return body
	}
	var payload any
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return InvalidJSONRedactedValue
	}
	masked := MaskSensitiveValue(payload)
	encoded, err := json.Marshal(masked)
	if err != nil {
		return InvalidJSONRedactedValue
	}
	return string(encoded)
}

// MaskSensitiveValue returns a deep copy with nested secret fields redacted.
func MaskSensitiveValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(current))
		for key, item := range current {
			if IsSensitiveField(key) {
				masked[key] = RedactedValue
				continue
			}
			masked[key] = MaskSensitiveValue(item)
		}
		return masked
	case []any:
		masked := make([]any, len(current))
		for i, item := range current {
			masked[i] = MaskSensitiveValue(item)
		}
		return masked
	default:
		return current
	}
}

// ContainsRedactedSensitiveValue detects display placeholders that must not be
// persisted as real credentials.
func ContainsRedactedSensitiveValue(value any) bool {
	switch current := value.(type) {
	case map[string]any:
		for key, item := range current {
			if IsSensitiveField(key) && IsRedactedValue(item) {
				return true
			}
			if ContainsRedactedSensitiveValue(item) {
				return true
			}
		}
	case []any:
		for _, item := range current {
			if ContainsRedactedSensitiveValue(item) {
				return true
			}
		}
	}
	return false
}

// RestoreRedactedSensitiveValues returns a deep copy of incoming. Secret
// placeholders are replaced with the corresponding stored value; placeholders
// without a stored counterpart are omitted instead of becoming credentials.
func RestoreRedactedSensitiveValues(incoming, stored any) any {
	switch current := incoming.(type) {
	case map[string]any:
		storedMap, _ := stored.(map[string]any)
		restored := make(map[string]any, len(current))
		for key, item := range current {
			storedItem, hasStored := storedMap[key]
			if IsSensitiveField(key) && IsRedactedValue(item) {
				if hasStored {
					restored[key] = cloneJSONValue(storedItem)
				}
				continue
			}
			restored[key] = RestoreRedactedSensitiveValues(item, storedItem)
		}
		return restored
	case []any:
		storedSlice, _ := stored.([]any)
		restored := make([]any, len(current))
		for i, item := range current {
			var storedItem any
			if i < len(storedSlice) {
				storedItem = storedSlice[i]
			}
			restored[i] = RestoreRedactedSensitiveValues(item, storedItem)
		}
		return restored
	default:
		return current
	}
}

func cloneJSONValue(value any) any {
	switch current := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(current))
		for key, item := range current {
			cloned[key] = cloneJSONValue(item)
		}
		return cloned
	case []any:
		cloned := make([]any, len(current))
		for i, item := range current {
			cloned[i] = cloneJSONValue(item)
		}
		return cloned
	default:
		return current
	}
}

func normalizeFieldName(field string) string {
	runes := []rune(strings.TrimSpace(field))
	var normalized strings.Builder
	for i, r := range runes {
		if r >= 'A' && r <= 'Z' {
			previousIsLowerOrDigit := i > 0 && ((runes[i-1] >= 'a' && runes[i-1] <= 'z') || (runes[i-1] >= '0' && runes[i-1] <= '9'))
			nextIsLower := i+1 < len(runes) && runes[i+1] >= 'a' && runes[i+1] <= 'z'
			if i > 0 && (previousIsLowerOrDigit || nextIsLower) {
				normalized.WriteByte('_')
			}
			normalized.WriteRune(r + ('a' - 'A'))
			continue
		}
		if r == '-' || r == '.' || r == ' ' {
			if normalized.Len() > 0 {
				normalized.WriteByte('_')
			}
			continue
		}
		normalized.WriteRune(r)
	}
	return strings.ToLower(normalized.String())
}

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
