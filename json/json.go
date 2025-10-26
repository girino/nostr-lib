package json

import (
	"encoding/json"
	"fmt"
)

// JsonEntity is the base interface for all JSON types
type JsonEntity interface {
	// Type returns the type name of this JSON entity
	Type() string

	// MarshalJSON implements json.Marshaler interface
	MarshalJSON() ([]byte, error)

	// UnmarshalJSON implements json.Unmarshaler interface
	UnmarshalJSON(data []byte) error
}

// NewJsonValue creates a new JsonValue from various types
func NewJsonValue(v interface{}) *JsonValue {
	jv := &JsonValue{}

	switch val := v.(type) {
	case int:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case int8:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case int16:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case int32:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case int64:
		jv.typeVal = "int"
		jv.IntVal = val
	case uint:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case uint8:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case uint16:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case uint32:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case uint64:
		jv.typeVal = "int"
		jv.IntVal = int64(val)
	case float32:
		jv.typeVal = "float"
		jv.FloatVal = float64(val)
	case float64:
		jv.typeVal = "float"
		jv.FloatVal = val
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

	return jv
}

// NewJsonList creates a new empty JsonList
func NewJsonList() *JsonList {
	return &JsonList{
		Items: make([]JsonEntity, 0),
	}
}

// NewJsonObject creates a new empty JsonObject
func NewJsonObject() *JsonObject {
	return &JsonObject{
		data: make(map[string]JsonEntity),
		keys: make([]string, 0),
	}
}

// Unmarshal parses JSON bytes into a JsonEntity
func Unmarshal(data []byte) (JsonEntity, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}

	return valueToEntity(v), nil
}

// valueToEntity converts a raw Go value to a JsonEntity
func valueToEntity(v interface{}) JsonEntity {
	switch val := v.(type) {
	case map[string]interface{}:
		obj := NewJsonObject()
		// Note: Go's map iteration order is random, so we need to sort by key
		// For now, we preserve whatever order the keys come in during iteration
		for k, vi := range val {
			obj.Set(k, valueToEntity(vi))
		}
		return obj
	case []interface{}:
		list := NewJsonList()
		for _, vi := range val {
			list.Append(valueToEntity(vi))
		}
		return list
	case float64:
		// JSON numbers are always unmarshaled as float64
		// Check if it's actually an integer
		if val == float64(int64(val)) {
			return &JsonValue{typeVal: "int", IntVal: int64(val)}
		}
		return &JsonValue{typeVal: "float", FloatVal: val}
	case string:
		return &JsonValue{typeVal: "string", StringVal: val}
	case bool:
		return &JsonValue{typeVal: "bool", BoolVal: val}
	case nil:
		return &JsonValue{typeVal: "null"}
	default:
		return &JsonValue{typeVal: "null"}
	}
}

// AsJsonEntity attempts to convert a value to JsonEntity or returns an error
func AsJsonEntity(v interface{}) (JsonEntity, error) {
	switch val := v.(type) {
	case JsonEntity:
		return val, nil
	case string:
		return NewJsonValue(val), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return NewJsonValue(val), nil
	case float32, float64:
		return NewJsonValue(val), nil
	case bool:
		return NewJsonValue(val), nil
	case nil:
		return NewJsonValue(nil), nil
	default:
		return nil, fmt.Errorf("cannot convert %T to JsonEntity", v)
	}
}

// Marshal is a convenience function that wraps json.Marshal for JsonEntity
func Marshal(v JsonEntity) ([]byte, error) {
	return v.MarshalJSON()
}

// MarshalIndent marshals a JsonEntity with indentation
func MarshalIndent(v JsonEntity, prefix, indent string) ([]byte, error) {
	data, err := v.MarshalJSON()
	if err != nil {
		return nil, err
	}

	// Parse the JSON to add indentation
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	return json.MarshalIndent(parsed, prefix, indent)
}
