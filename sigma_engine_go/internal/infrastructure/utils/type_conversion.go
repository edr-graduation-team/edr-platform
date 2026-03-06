package utils

import (
	"fmt"
	"strconv"
)

// ToString converts any type to string.
// Returns empty string for nil values.
func ToString(v interface{}) (string, error) {
	if v == nil {
		return "", nil
	}

	switch val := v.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", val), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val), nil
	case float32, float64:
		return fmt.Sprintf("%g", val), nil
	case bool:
		return strconv.FormatBool(val), nil
	default:
		return fmt.Sprintf("%v", val), nil
	}
}

// ToFloat64 converts any type to float64.
// Returns an error if conversion is not possible.
func ToFloat64(v interface{}) (float64, error) {
	if v == nil {
		return 0, fmt.Errorf("cannot convert nil to float64")
	}

	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int8:
		return float64(val), nil
	case int16:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case uint:
		return float64(val), nil
	case uint8:
		return float64(val), nil
	case uint16:
		return float64(val), nil
	case uint32:
		return float64(val), nil
	case uint64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// ToInt64 converts any type to int64.
// Returns an error if conversion is not possible.
func ToInt64(v interface{}) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("cannot convert nil to int64")
	}

	switch val := v.(type) {
	case int64:
		return val, nil
	case int:
		return int64(val), nil
	case int8:
		return int64(val), nil
	case int16:
		return int64(val), nil
	case int32:
		return int64(val), nil
	case uint:
		return int64(val), nil
	case uint8:
		return int64(val), nil
	case uint16:
		return int64(val), nil
	case uint32:
		return int64(val), nil
	case uint64:
		return int64(val), nil
	case float32:
		return int64(val), nil
	case float64:
		return int64(val), nil
	case string:
		return strconv.ParseInt(val, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", v)
	}
}

// ToBool converts any type to bool.
// Returns an error if conversion is not possible.
func ToBool(v interface{}) (bool, error) {
	if v == nil {
		return false, fmt.Errorf("cannot convert nil to bool")
	}

	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(val)
	case int, int8, int16, int32, int64:
		// Non-zero integers are true
		return val != 0, nil
	case uint, uint8, uint16, uint32, uint64:
		return val != 0, nil
	case float32, float64:
		// Non-zero floats are true
		return val != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

// ToStringSlice converts any type to []string.
// Returns an error if conversion is not possible.
func ToStringSlice(v interface{}) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot convert nil to []string")
	}

	switch val := v.(type) {
	case []string:
		return val, nil
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			str, err := ToString(item)
			if err != nil {
				return nil, fmt.Errorf("error converting array element: %w", err)
			}
			result = append(result, str)
		}
		return result, nil
	case string:
		return []string{val}, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to []string", v)
	}
}

