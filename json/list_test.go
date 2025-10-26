package json

import (
	"encoding/json"
	"testing"
)

func TestJsonList_Append(t *testing.T) {
	list := NewJsonList()

	if list.Length() != 0 {
		t.Error("expected empty list to have length 0")
	}

	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue("hello"))

	if list.Length() != 2 {
		t.Errorf("expected length 2, got %d", list.Length())
	}
}

func TestJsonList_Insert(t *testing.T) {
	list := NewJsonList()

	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(3))

	err := list.Insert(1, NewJsonValue(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if list.Length() != 3 {
		t.Errorf("expected length 3, got %d", list.Length())
	}

	val, err := list.Get(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	intVal, _ := val.(*JsonValue).GetInt()
	if intVal != 2 {
		t.Errorf("expected value 2, got %d", intVal)
	}
}

func TestJsonList_Get(t *testing.T) {
	list := NewJsonList()
	list.Append(NewJsonValue("first"))
	list.Append(NewJsonValue("second"))

	val, err := list.Get(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "first" {
		t.Errorf("expected 'first', got %s", strVal)
	}

	_, err = list.Get(10)
	if err == nil {
		t.Error("expected error for out of range index")
	}
}

func TestJsonList_Remove(t *testing.T) {
	list := NewJsonList()
	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(2))
	list.Append(NewJsonValue(3))

	err := list.Remove(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if list.Length() != 2 {
		t.Errorf("expected length 2, got %d", list.Length())
	}

	val, _ := list.Get(0)
	intVal, _ := val.(*JsonValue).GetInt()
	if intVal != 1 {
		t.Errorf("expected 1, got %d", intVal)
	}

	val, _ = list.Get(1)
	intVal, _ = val.(*JsonValue).GetInt()
	if intVal != 3 {
		t.Errorf("expected 3, got %d", intVal)
	}
}

func TestJsonList_IsEmpty(t *testing.T) {
	list := NewJsonList()

	if !list.IsEmpty() {
		t.Error("expected empty list to be empty")
	}

	list.Append(NewJsonValue(1))
	if list.IsEmpty() {
		t.Error("expected non-empty list to not be empty")
	}
}

func TestJsonList_Clear(t *testing.T) {
	list := NewJsonList()
	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(2))

	list.Clear()

	if list.Length() != 0 {
		t.Errorf("expected length 0 after clear, got %d", list.Length())
	}
	if !list.IsEmpty() {
		t.Error("expected list to be empty after clear")
	}
}

func TestJsonList_MarshalJSON(t *testing.T) {
	list := NewJsonList()
	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(2))
	list.Append(NewJsonValue(3))

	data, err := list.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse it back to verify
	var parsed []int
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(parsed) != 3 {
		t.Errorf("expected length 3, got %d", len(parsed))
	}
}

func TestJsonList_UnmarshalJSON(t *testing.T) {
	input := `[1, 2, 3]`
	list := &JsonList{}

	err := list.UnmarshalJSON([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if list.Length() != 3 {
		t.Errorf("expected length 3, got %d", list.Length())
	}

	val, _ := list.Get(0)
	intVal, _ := val.(*JsonValue).GetInt()
	if intVal != 1 {
		t.Errorf("expected 1, got %d", intVal)
	}
}

func TestJsonList_ComplexTypes(t *testing.T) {
	list := NewJsonList()

	// Add a nested object
	obj := NewJsonObject()
	obj.Set("key", NewJsonValue("value"))
	list.Append(obj)

	// Add a nested list
	nestedList := NewJsonList()
	nestedList.Append(NewJsonValue("nested"))
	list.Append(nestedList)

	if list.Length() != 2 {
		t.Errorf("expected length 2, got %d", list.Length())
	}

	// Verify we can marshal it
	data, err := list.MarshalJSON()
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

func TestJsonList_At(t *testing.T) {
	list := NewJsonList()
	list.Append(NewJsonValue("first"))
	list.Append(NewJsonValue("second"))

	val := list.At(0)
	strVal, _ := val.(*JsonValue).GetString()
	if strVal != "first" {
		t.Errorf("expected 'first', got %s", strVal)
	}

	// Out of range should return nil
	val = list.At(10)
	if val != nil {
		t.Error("expected nil for out of range")
	}
}
