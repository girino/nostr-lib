package json

import (
	"encoding/json"
	"fmt"
)

// JsonList is an ordered list of JsonEntity elements
type JsonList struct {
	Items []JsonEntity
}

// Type returns the string "list"
func (jl *JsonList) Type() string {
	return "list"
}

// MarshalJSON implements json.Marshaler interface
func (jl *JsonList) MarshalJSON() ([]byte, error) {
	return json.Marshal(jl.Items)
}

// UnmarshalJSON implements json.Unmarshaler interface
func (jl *JsonList) UnmarshalJSON(data []byte) error {
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	jl.Items = make([]JsonEntity, 0, len(raw))
	for _, item := range raw {
		jl.Items = append(jl.Items, valueToEntity(item))
	}

	return nil
}

// Append adds an item to the end of the list
func (jl *JsonList) Append(item JsonEntity) {
	jl.Items = append(jl.Items, item)
}

// Insert inserts an item at the specified index
func (jl *JsonList) Insert(index int, item JsonEntity) error {
	if index < 0 || index > len(jl.Items) {
		return fmt.Errorf("index out of range: %d", index)
	}

	jl.Items = append(jl.Items[:index], append([]JsonEntity{item}, jl.Items[index:]...)...)
	return nil
}

// Get returns the item at the specified index
func (jl *JsonList) Get(index int) (JsonEntity, error) {
	if index < 0 || index >= len(jl.Items) {
		return nil, fmt.Errorf("index out of range: %d", index)
	}

	return jl.Items[index], nil
}

// Remove removes the item at the specified index
func (jl *JsonList) Remove(index int) error {
	if index < 0 || index >= len(jl.Items) {
		return fmt.Errorf("index out of range: %d", index)
	}

	jl.Items = append(jl.Items[:index], jl.Items[index+1:]...)
	return nil
}

// Length returns the number of items in the list
func (jl *JsonList) Length() int {
	return len(jl.Items)
}

// IsEmpty returns true if the list is empty
func (jl *JsonList) IsEmpty() bool {
	return len(jl.Items) == 0
}

// Clear removes all items from the list
func (jl *JsonList) Clear() {
	jl.Items = jl.Items[:0]
}

// ToSlice returns the underlying slice of JsonEntity
func (jl *JsonList) ToSlice() []JsonEntity {
	return jl.Items
}

// At returns the item at the specified index (convenience method)
func (jl *JsonList) At(index int) JsonEntity {
	if index < 0 || index >= len(jl.Items) {
		return nil
	}
	return jl.Items[index]
}
