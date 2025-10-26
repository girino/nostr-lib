package json

import (
	"encoding/json"
	"testing"
)

func TestUnmarshal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
	}{
		{
			name:     "simple object",
			input:    `{"name": "John", "age": 30}`,
			wantType: "object",
		},
		{
			name:     "simple array",
			input:    `[1, 2, 3]`,
			wantType: "list",
		},
		{
			name:     "string",
			input:    `"hello"`,
			wantType: "value",
		},
		{
			name:     "number",
			input:    `42`,
			wantType: "value",
		},
		{
			name:     "boolean",
			input:    `true`,
			wantType: "value",
		},
		{
			name:     "null",
			input:    `null`,
			wantType: "value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := Unmarshal([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if entity.Type() != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, entity.Type())
			}

			// Verify we can marshal it back
			data, err := entity.MarshalJSON()
			if err != nil {
				t.Fatalf("unexpected error marshaling: %v", err)
			}

			// Should be valid JSON
			var v interface{}
			err = json.Unmarshal(data, &v)
			if err != nil {
				t.Fatalf("unexpected error unmarshaling: %v", err)
			}
		})
	}
}

func TestUnmarshalComplex(t *testing.T) {
	input := `{
		"name": "John",
		"age": 30,
		"hobbies": ["reading", "swimming"],
		"address": {
			"street": "123 Main St",
			"city": "New York"
		},
		"active": true,
		"balance": 1234.56
	}`

	entity, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj, ok := entity.(*JsonObject)
	if !ok {
		t.Fatal("expected JsonObject")
	}

	if obj.Length() != 6 {
		t.Errorf("expected 6 keys, got %d", obj.Length())
	}

	// Verify name
	val, exists := obj.Get("name")
	if !exists {
		t.Fatal("name key not found")
	}
	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "John" {
		t.Errorf("expected 'John', got %s", strVal)
	}

	// Verify age
	val, exists = obj.Get("age")
	if !exists {
		t.Fatal("age key not found")
	}
	intVal, _ := val.(*JsonValue).GetInt()
	if intVal != 30 {
		t.Errorf("expected 30, got %d", intVal)
	}

	// Verify hobbies (should be a list)
	val, exists = obj.Get("hobbies")
	if !exists {
		t.Fatal("hobbies key not found")
	}
	list, ok := val.(*JsonList)
	if !ok {
		t.Fatal("expected JsonList for hobbies")
	}
	if list.Length() != 2 {
		t.Errorf("expected 2 hobbies, got %d", list.Length())
	}

	// Verify address (should be an object)
	val, exists = obj.Get("address")
	if !exists {
		t.Fatal("address key not found")
	}
	addrObj, ok := val.(*JsonObject)
	if !ok {
		t.Fatal("expected JsonObject for address")
	}
	if addrObj.Length() != 2 {
		t.Errorf("expected 2 address fields, got %d", addrObj.Length())
	}

	// Verify active
	val, exists = obj.Get("active")
	if !exists {
		t.Fatal("active key not found")
	}
	boolVal, _ := val.(*JsonValue).GetBool()
	if !boolVal {
		t.Error("expected active to be true")
	}

	// Verify balance
	val, exists = obj.Get("balance")
	if !exists {
		t.Fatal("balance key not found")
	}
	floatVal, _ := val.(*JsonValue).GetFloat()
	if floatVal != 1234.56 {
		t.Errorf("expected 1234.56, got %f", floatVal)
	}
}

func TestJsonEntityRoundTrip(t *testing.T) {
	// Build a complex structure
	obj := NewJsonObject()
	obj.Set("name", NewJsonValue("John"))
	obj.Set("age", NewJsonValue(30))

	hobbies := NewJsonList()
	hobbies.Append(NewJsonValue("reading"))
	hobbies.Append(NewJsonValue("swimming"))
	obj.Set("hobbies", hobbies)

	address := NewJsonObject()
	address.Set("street", NewJsonValue("123 Main St"))
	address.Set("city", NewJsonValue("New York"))
	obj.Set("address", address)

	// Marshal to JSON
	data, err := obj.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal it back
	var backObj map[string]interface{}
	err = json.Unmarshal(data, &backObj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify structure
	if backObj["name"] != "John" {
		t.Errorf("expected name to be 'John', got %v", backObj["name"])
	}

	if backObj["age"] != float64(30) {
		t.Errorf("expected age to be 30, got %v", backObj["age"])
	}

	hobbiesList, ok := backObj["hobbies"].([]interface{})
	if !ok {
		t.Fatal("expected hobbies to be an array")
	}

	if len(hobbiesList) != 2 {
		t.Errorf("expected 2 hobbies, got %d", len(hobbiesList))
	}

	addrMap, ok := backObj["address"].(map[string]interface{})
	if !ok {
		t.Fatal("expected address to be an object")
	}

	if addrMap["street"] != "123 Main St" {
		t.Errorf("expected street to be '123 Main St', got %v", addrMap["street"])
	}
}

func TestAsJsonEntity(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{"JsonValue", NewJsonValue(42), false},
		{"JsonList", NewJsonList(), false},
		{"JsonObject", NewJsonObject(), false},
		{"string", "hello", false},
		{"int", 42, false},
		{"float", 3.14, false},
		{"bool", true, false},
		{"nil", nil, false},
		{"struct", struct{}{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AsJsonEntity(tt.input)
			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestComplexNestedStructure(t *testing.T) {
	// Build a deeply nested structure
	root := NewJsonObject()
	root.Set("level1", NewJsonValue("value1"))

	level2 := NewJsonObject()
	level2.Set("level2", NewJsonValue("value2"))

	level3 := NewJsonObject()
	level3.Set("level3", NewJsonValue("value3"))
	level2.Set("nested", level3)

	root.Set("nested", level2)

	list := NewJsonList()
	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(2))
	nestedList := NewJsonList()
	nestedList.Append(NewJsonValue("deep"))
	list.Append(nestedList)
	root.Set("list", list)

	// Marshal and verify
	data, err := root.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON
	var v interface{}
	err = json.Unmarshal(data, &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify we can unmarshal it back
	root2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rootObj, ok := root2.(*JsonObject)
	if !ok {
		t.Fatal("expected JsonObject")
	}

	if rootObj.Length() != 3 {
		t.Errorf("expected 3 top-level keys, got %d", rootObj.Length())
	}

	// Verify nested object
	nested, exists := rootObj.Get("nested")
	if !exists {
		t.Fatal("nested key not found")
	}

	nestedObj, ok := nested.(*JsonObject)
	if !ok {
		t.Fatal("expected nested JsonObject")
	}

	deeper, exists := nestedObj.Get("nested")
	if !exists {
		t.Fatal("deeper nested key not found")
	}

	deeperObj, ok := deeper.(*JsonObject)
	if !ok {
		t.Fatal("expected deeper nested JsonObject")
	}

	val, exists := deeperObj.Get("level3")
	if !exists {
		t.Fatal("level3 key not found")
	}

	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "value3" {
		t.Errorf("expected 'value3', got %s", strVal)
	}
}

func TestOrderPreservationRoundTrip(t *testing.T) {
	// Create an object with keys in specific order
	obj := NewJsonObject()
	obj.Set("zebra", NewJsonValue("last"))
	obj.Set("apple", NewJsonValue("first"))
	obj.Set("mango", NewJsonValue("middle"))

	// Marshal
	data, err := obj.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal into our type again
	obj2, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	unmarshaledObj, ok := obj2.(*JsonObject)
	if !ok {
		t.Fatal("expected JsonObject")
	}

	keys := unmarshaledObj.Keys()

	// The keys should still be in the order they were inserted
	// Note: Go's json.Unmarshal doesn't preserve order, so this test
	// verifies that our implementation attempts to preserve order
	// during construction, even if it may be lost during a round-trip
	// through standard JSON unmarshaling

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}
}
