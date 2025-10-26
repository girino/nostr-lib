package json

import (
	"testing"
)

func TestJsonValue_Creation(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"int", 42, "int"},
		{"float", 3.14, "float"},
		{"string", "hello", "string"},
		{"bool", true, "bool"},
		{"nil", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJsonValue(tt.input)
			if v.GetType() != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, v.GetType())
			}
		})
	}
}

func TestJsonValue_GetInt(t *testing.T) {
	v := NewJsonValue(int64(42))
	val, ok := v.GetInt()
	if !ok {
		t.Error("expected ok to be true for int")
	}
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	v2 := NewJsonValue("not an int")
	_, ok = v2.GetInt()
	if ok {
		t.Error("expected ok to be false for non-int")
	}
}

func TestJsonValue_GetFloat(t *testing.T) {
	v := NewJsonValue(3.14)
	val, ok := v.GetFloat()
	if !ok {
		t.Error("expected ok to be true for float")
	}
	if val != 3.14 {
		t.Errorf("expected 3.14, got %f", val)
	}
}

func TestJsonValue_GetString(t *testing.T) {
	v := NewJsonValue("hello")
	val, ok := v.GetString()
	if !ok {
		t.Error("expected ok to be true for string")
	}
	if val != "hello" {
		t.Errorf("expected hello, got %s", val)
	}
}

func TestJsonValue_GetBool(t *testing.T) {
	v := NewJsonValue(true)
	val, ok := v.GetBool()
	if !ok {
		t.Error("expected ok to be true for bool")
	}
	if val != true {
		t.Error("expected true, got false")
	}
}

func TestJsonValue_IsNull(t *testing.T) {
	v := NewJsonValue(nil)
	if !v.IsNull() {
		t.Error("expected IsNull to return true")
	}

	v2 := NewJsonValue("not null")
	if v2.IsNull() {
		t.Error("expected IsNull to return false")
	}
}

func TestJsonValue_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{"int", int64(42), "42"},
		{"float", 3.14, "3.14"},
		{"string", "hello", `"hello"`},
		{"bool", true, "true"},
		{"null", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewJsonValue(tt.value)
			data, err := v.MarshalJSON()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(data) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(data))
			}
		})
	}
}

func TestJsonValue_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{"int", "42", int64(42)},
		{"float", "3.14", 3.14},
		{"string", `"hello"`, "hello"},
		{"bool", "true", true},
		{"null", "null", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &JsonValue{}
			err := v.UnmarshalJSON([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if v.GetType() == "int" {
				val, _ := v.GetInt()
				if val != tt.expected {
					t.Errorf("expected %v, got %d", tt.expected, val)
				}
			} else if v.GetType() == "float" {
				val, _ := v.GetFloat()
				if val != tt.expected {
					t.Errorf("expected %v, got %f", tt.expected, val)
				}
			} else if v.GetType() == "string" {
				val, _ := v.GetString()
				if val != tt.expected {
					t.Errorf("expected %v, got %s", tt.expected, val)
				}
			} else if v.GetType() == "bool" {
				val, _ := v.GetBool()
				if val != tt.expected {
					t.Errorf("expected %v, got %t", tt.expected, val)
				}
			}
		})
	}
}

func TestJsonValue_ToInterface(t *testing.T) {
	v := NewJsonValue(42)
	if val := v.ToInterface(); val != int64(42) {
		t.Errorf("expected 42, got %v", val)
	}

	v2 := NewJsonValue("hello")
	if val := v2.ToInterface(); val != "hello" {
		t.Errorf("expected hello, got %v", val)
	}
}

func TestJsonValue_Equal(t *testing.T) {
	v1 := NewJsonValue(42)
	v2 := NewJsonValue(42)
	v3 := NewJsonValue(43)

	if !v1.Equal(v2) {
		t.Error("expected v1 and v2 to be equal")
	}
	if v1.Equal(v3) {
		t.Error("expected v1 and v3 to be different")
	}
}

func TestJsonValue_SetMethods(t *testing.T) {
	v := &JsonValue{}

	v.SetInt(42)
	if val, ok := v.GetInt(); !ok || val != 42 {
		t.Error("SetInt failed")
	}

	v.SetFloat(3.14)
	if val, ok := v.GetFloat(); !ok || val != 3.14 {
		t.Error("SetFloat failed")
	}

	v.SetString("test")
	if val, ok := v.GetString(); !ok || val != "test" {
		t.Error("SetString failed")
	}

	v.SetBool(true)
	if val, ok := v.GetBool(); !ok || !val {
		t.Error("SetBool failed")
	}

	v.SetNull()
	if !v.IsNull() {
		t.Error("SetNull failed")
	}
}
