package runtimeconfig

import "math"

// NonNegativeSetting returns a configured integer when it is zero or greater.
func NonNegativeSetting(value map[string]any, key string, fallback int) int {
	if got, ok := IntSetting(value[key]); ok && got >= 0 {
		return got
	}
	return fallback
}

// PositiveSetting returns a configured integer when it is greater than zero.
func PositiveSetting(value map[string]any, key string, fallback int) int {
	if got, ok := PositiveInt(value[key]); ok {
		return got
	}
	return fallback
}

// PositiveOrDefault converts a non-positive static configuration value to its
// safe positive fallback.
func PositiveOrDefault(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

// PositiveInt converts supported JSON/config numeric values to a positive int.
func PositiveInt(value any) (int, bool) {
	got, ok := IntSetting(value)
	return got, ok && got > 0
}

// IntSetting converts the numeric forms produced by configuration decoding
// without allowing overflow or fractional float64 values.
func IntSetting(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		if uint64(v) > uint64(math.MaxInt) {
			return 0, false
		}
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		if v > uint64(math.MaxInt) {
			return 0, false
		}
		return int(v), true
	case float64:
		if math.Trunc(v) != v || v > float64(math.MaxInt) || v < float64(math.MinInt) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}
