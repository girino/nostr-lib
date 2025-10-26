package json

import (
	"encoding/json"
	"testing"
)

func TestJsonObject_SetGet(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("name", NewJsonValue("John"))
	obj.Set("age", NewJsonValue(30))

	val, exists := obj.Get("name")
	if !exists {
		t.Error("expected name to exist")
	}

	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "John" {
		t.Errorf("expected 'John', got %s", strVal)
	}

	val, exists = obj.Get("missing")
	if exists {
		t.Error("expected missing key to not exist")
	}
}

func TestJsonObject_Delete(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("key1", NewJsonValue("value1"))
	obj.Set("key2", NewJsonValue("value2"))

	if obj.Length() != 2 {
		t.Errorf("expected length 2, got %d", obj.Length())
	}

	obj.Delete("key1")

	if obj.Length() != 1 {
		t.Errorf("expected length 1, got %d", obj.Length())
	}

	if obj.Has("key1") {
		t.Error("expected key1 to be deleted")
	}

	if !obj.Has("key2") {
		t.Error("expected key2 to still exist")
	}
}

func TestJsonObject_Has(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("key", NewJsonValue("value"))

	if !obj.Has("key") {
		t.Error("expected Has to return true for existing key")
	}

	if obj.Has("missing") {
		t.Error("expected Has to return false for missing key")
	}
}

func TestJsonObject_Keys(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("a", NewJsonValue(1))
	obj.Set("b", NewJsonValue(2))
	obj.Set("c", NewJsonValue(3))

	keys := obj.Keys()

	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	// Check that keys maintain insertion order
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("keys not in insertion order: %v", keys)
	}
}

func TestJsonObject_Length(t *testing.T) {
	obj := NewJsonObject()

	if obj.Length() != 0 {
		t.Error("expected empty object to have length 0")
	}

	obj.Set("key", NewJsonValue("value"))

	if obj.Length() != 1 {
		t.Errorf("expected length 1, got %d", obj.Length())
	}
}

func TestJsonObject_IsEmpty(t *testing.T) {
	obj := NewJsonObject()

	if !obj.IsEmpty() {
		t.Error("expected empty object to be empty")
	}

	obj.Set("key", NewJsonValue("value"))

	if obj.IsEmpty() {
		t.Error("expected non-empty object to not be empty")
	}
}

func TestJsonObject_Clear(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("key1", NewJsonValue("value1"))
	obj.Set("key2", NewJsonValue("value2"))

	obj.Clear()

	if obj.Length() != 0 {
		t.Errorf("expected length 0 after clear, got %d", obj.Length())
	}

	if !obj.IsEmpty() {
		t.Error("expected object to be empty after clear")
	}
}

func TestJsonObject_MarshalJSON(t *testing.T) {
	obj := NewJsonObject()
	obj.Set("name", NewJsonValue("John"))
	obj.Set("age", NewJsonValue(30))

	data, err := obj.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse it back to verify
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if parsed["name"] != "John" {
		t.Errorf("expected name to be John, got %v", parsed["name"])
	}

	if parsed["age"] != float64(30) {
		t.Errorf("expected age to be 30, got %v", parsed["age"])
	}
}

func TestJsonObject_UnmarshalJSON(t *testing.T) {
	input := `{"name": "John", "age": 30}`
	obj := &JsonObject{}

	err := obj.UnmarshalJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obj.Length() != 2 {
		t.Errorf("expected length 2, got %d", obj.Length())
	}

	val, exists := obj.Get("name")
	if !exists {
		t.Fatal("expected name to exist")
	}

	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "John" {
		t.Errorf("expected 'John', got %s", strVal)
	}
}

func TestJsonObject_OrderPreservation(t *testing.T) {
	obj := NewJsonObject()

	// Set keys in specific order
	obj.Set("z", NewJsonValue(1))
	obj.Set("a", NewJsonValue(2))
	obj.Set("m", NewJsonValue(3))

	keys := obj.Keys()

	// Keys should maintain insertion order
	if keys[0] != "z" || keys[1] != "a" || keys[2] != "m" {
		t.Errorf("keys not in insertion order: %v", keys)
	}

	// Marshal and verify order is preserved
	data, err := obj.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Re-parse to verify order
	obj2 := &JsonObject{}
	err = obj2.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	keys2 := obj2.Keys()
	if len(keys2) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys2))
	}

	// Verify the keys are in the same order after round-trip
	if keys2[0] != "z" || keys2[1] != "a" || keys2[2] != "m" {
		t.Errorf("keys not in correct order after unmarshaling: %v (expected [z a m])", keys2)
	}
}

func TestJsonObject_ForEach(t *testing.T) {
	obj := NewJsonObject()
	obj.Set("a", NewJsonValue(1))
	obj.Set("b", NewJsonValue(2))
	obj.Set("c", NewJsonValue(3))

	visited := make([]string, 0)
	obj.ForEach(func(key string, value JsonEntity) bool {
		visited = append(visited, key)
		return true
	})

	if len(visited) != 3 {
		t.Errorf("expected 3 keys visited, got %d", len(visited))
	}

	if visited[0] != "a" || visited[1] != "b" || visited[2] != "c" {
		t.Errorf("keys not in expected order: %v", visited)
	}
}

func TestJsonObject_ForEachEarlyExit(t *testing.T) {
	obj := NewJsonObject()
	obj.Set("a", NewJsonValue(1))
	obj.Set("b", NewJsonValue(2))
	obj.Set("c", NewJsonValue(3))

	count := 0
	obj.ForEach(func(key string, value JsonEntity) bool {
		count++
		return count < 2 // Exit after 2 iterations
	})

	if count != 2 {
		t.Errorf("expected 2 iterations, got %d", count)
	}
}

func TestJsonObject_UpdateExistingKey(t *testing.T) {
	obj := NewJsonObject()

	obj.Set("key", NewJsonValue("original"))
	obj.Set("key", NewJsonValue("updated"))

	if obj.Length() != 1 {
		t.Errorf("expected length 1, got %d", obj.Length())
	}

	val, _ := obj.Get("key")
	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "updated" {
		t.Errorf("expected 'updated', got %s", strVal)
	}

	// Keys list should still have only one entry
	keys := obj.Keys()
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestJsonObject_GetOrNil(t *testing.T) {
	obj := NewJsonObject()
	obj.Set("key", NewJsonValue("value"))

	val := obj.GetOrNil("key")
	if val == nil {
		t.Error("expected non-nil value")
	}

	val2 := obj.GetOrNil("missing")
	if val2 != nil {
		t.Error("expected nil value for missing key")
	}
}

func TestJsonObject_NestedStructures(t *testing.T) {
	obj := NewJsonObject()

	// Add a nested object
	nestedObj := NewJsonObject()
	nestedObj.Set("nested_key", NewJsonValue("nested_value"))
	obj.Set("nested", nestedObj)

	// Add a list
	list := NewJsonList()
	list.Append(NewJsonValue("item1"))
	list.Append(NewJsonValue("item2"))
	obj.Set("list", list)

	if obj.Length() != 2 {
		t.Errorf("expected length 2, got %d", obj.Length())
	}

	// Marshal it to verify
	data, err := obj.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be valid JSON
	var v interface{}
	err = json.Unmarshal(data, &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
