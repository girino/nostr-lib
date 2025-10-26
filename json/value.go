package json

import (
	"encoding/json"
	"fmt"
)

// JsonValue represents a JSON value that can be an integer, float, string, bool, or null
type JsonValue struct {
	typeVal   string
	IntVal    int64
	FloatVal  float64
	StringVal string
	BoolVal   bool
}

// Type returns the string "value"
func (jv *JsonValue) Type() string {
	return "value"
}

// GetType returns the type of the value as a string
func (jv *JsonValue) GetType() string {
	return jv.typeVal
}

// MarshalJSON implements json.Marshaler interface
func (jv *JsonValue) MarshalJSON() ([]byte, error) {
	switch jv.typeVal {
	case "int":
		return json.Marshal(jv.IntVal)
	case "float":
		return json.Marshal(jv.FloatVal)
	case "string":
		return json.Marshal(jv.StringVal)
	case "bool":
		return json.Marshal(jv.BoolVal)
	case "null":
		return []byte("null"), nil
	default:
		return []byte("null"), nil
	}
}

// UnmarshalJSON implements json.Unmarshaler interface
func (jv *JsonValue) UnmarshalJSON(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch val := v.(type) {
	case float64:
		// JSON numbers are always float64, check if it's an integer
		if val == float64(int64(val)) {
			jv.typeVal = "int"
			jv.IntVal = int64(val)
		} else {
			jv.typeVal = "float"
			jv.FloatVal = val
		}
	case string:
		jv.typeVal = "string"
		jv.StringVal = val
	case bool:
		jv.typeVal = "bool"
		jv.BoolVal = val
	case nil:
		jv.typeVal = "null"
	default:
		jv.typeVal = "null"
	}

	return nil
}

// GetInt returns the integer value and true if the value is an integer
func (jv *JsonValue) GetInt() (int64, bool) {
	if jv.typeVal == "int" {
		return jv.IntVal, true
	}
	return 0, false
}

// GetFloat returns the float value and true if the value is a float
func (jv *JsonValue) GetFloat() (float64, bool) {
	if jv.typeVal == "float" {
		return jv.FloatVal, true
	}
	return 0, false
}

// GetString returns the string value and true if the value is a string
func (jv *JsonValue) GetString() (string, bool) {
	if jv.typeVal == "string" {
		return jv.StringVal, true
	}
	return "", false
}

// GetBool returns the bool value and true if the value is a bool
func (jv *JsonValue) GetBool() (bool, bool) {
	if jv.typeVal == "bool" {
		return jv.BoolVal, true
	}
	return false, false
}

// IsNull returns true if the value is null
func (jv *JsonValue) IsNull() bool {
	return jv.typeVal == "null"
}

// SetInt sets the value to an integer
func (jv *JsonValue) SetInt(val int64) {
	jv.typeVal = "int"
	jv.IntVal = val
}

// SetFloat sets the value to a float
func (jv *JsonValue) SetFloat(val float64) {
	jv.typeVal = "float"
	jv.FloatVal = val
}

// SetString sets the value to a string
func (jv *JsonValue) SetString(val string) {
	jv.typeVal = "string"
	jv.StringVal = val
}

// SetBool sets the value to a bool
func (jv *JsonValue) SetBool(val bool) {
	jv.typeVal = "bool"
	jv.BoolVal = val
}

// SetNull sets the value to null
func (jv *JsonValue) SetNull() {
	jv.typeVal = "null"
}

// ToInterface returns the underlying Go value
func (jv *JsonValue) ToInterface() interface{} {
	switch jv.typeVal {
	case "int":
		return jv.IntVal
	case "float":
		return jv.FloatVal
	case "string":
		return jv.StringVal
	case "bool":
		return jv.BoolVal
	case "null":
		return nil
	default:
		return nil
	}
}

// String returns a string representation of the value
func (jv *JsonValue) String() string {
	switch jv.typeVal {
	case "int":
		return fmt.Sprintf("%d", jv.IntVal)
	case "float":
		return fmt.Sprintf("%g", jv.FloatVal)
	case "string":
		return jv.StringVal
	case "bool":
		return fmt.Sprintf("%t", jv.BoolVal)
	case "null":
		return "null"
	default:
		return "null"
	}
}

// Equal compares two JsonValues for equality
func (jv *JsonValue) Equal(other *JsonValue) bool {
	if jv.typeVal != other.typeVal {
		return false
	}

	switch jv.typeVal {
	case "int":
		return jv.IntVal == other.IntVal
	case "float":
		return jv.FloatVal == other.FloatVal
	case "string":
		return jv.StringVal == other.StringVal
	case "bool":
		return jv.BoolVal == other.BoolVal
	case "null":
		return true
	default:
		return false
	}
}
