package json

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// JsonObject is an ordered map of string keys to JsonEntity values
// It uses a map for O(1) lookups and a slice to track insertion order
type JsonObject struct {
	data map[string]JsonEntity
	keys []string
}

// Type returns the string "object"
func (jo *JsonObject) Type() string {
	return "object"
}

// MarshalJSON implements json.Marshaler interface
// Marshals keys in insertion order
func (jo *JsonObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	first := true
	for _, key := range jo.keys {
		if val, exists := jo.data[key]; exists {
			if !first {
				buf.WriteByte(',')
			}
			first = false

			// Write key
			keyBytes, _ := json.Marshal(key)
			buf.Write(keyBytes)
			buf.WriteByte(':')

			// Write value
			valBytes, err := val.MarshalJSON()
			if err != nil {
				return nil, err
			}
			buf.Write(valBytes)
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON implements json.Unmarshaler interface
func (jo *JsonObject) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))

	// Read the opening brace
	tok, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected '{', got %v", tok)
	}

	jo.data = make(map[string]JsonEntity)
	jo.keys = make([]string, 0)

	// Read key-value pairs in order
	for decoder.More() {
		// Read key
		tok, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T", tok)
		}

		// Peek at the next token to determine the value type
		// Since we can't unpeek, we need to decode based on what we see
		var val JsonEntity

		// Use RawMessage to capture the value
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}

		// Now parse the raw value
		val, err = Unmarshal(raw)
		if err != nil {
			return err
		}

		jo.keys = append(jo.keys, key)
		jo.data[key] = val
	}

	// Read closing brace
	tok, err = decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '}' {
		return fmt.Errorf("expected '}', got %v", tok)
	}

	return nil
}

// Set sets a key-value pair in the object
// If the key already exists, it is updated but insertion order is not changed
// If the key is new, it is added at the end
func (jo *JsonObject) Set(key string, value JsonEntity) {
	if _, exists := jo.data[key]; !exists {
		jo.keys = append(jo.keys, key)
	}
	jo.data[key] = value
}

// Get returns the value for the given key
func (jo *JsonObject) Get(key string) (JsonEntity, bool) {
	val, exists := jo.data[key]
	return val, exists
}

// GetOrNil returns the value for the given key, or nil if it doesn't exist
func (jo *JsonObject) GetOrNil(key string) JsonEntity {
	if val, exists := jo.data[key]; exists {
		return val
	}
	return nil
}

// Delete removes a key-value pair from the object
func (jo *JsonObject) Delete(key string) bool {
	if _, exists := jo.data[key]; !exists {
		return false
	}

	delete(jo.data, key)

	// Remove from keys slice
	for i, k := range jo.keys {
		if k == key {
			jo.keys = append(jo.keys[:i], jo.keys[i+1:]...)
			break
		}
	}

	return true
}

// Has returns true if the key exists in the object
func (jo *JsonObject) Has(key string) bool {
	_, exists := jo.data[key]
	return exists
}

// Keys returns all keys in insertion order
func (jo *JsonObject) Keys() []string {
	keys := make([]string, len(jo.keys))
	copy(keys, jo.keys)
	return keys
}

// Length returns the number of key-value pairs in the object
func (jo *JsonObject) Length() int {
	return len(jo.data)
}

// IsEmpty returns true if the object has no key-value pairs
func (jo *JsonObject) IsEmpty() bool {
	return len(jo.data) == 0
}

// Clear removes all key-value pairs from the object
func (jo *JsonObject) Clear() {
	jo.data = make(map[string]JsonEntity)
	jo.keys = jo.keys[:0]
}

// ToMap returns a copy of the underlying data map
func (jo *JsonObject) ToMap() map[string]JsonEntity {
	result := make(map[string]JsonEntity)
	for k, v := range jo.data {
		result[k] = v
	}
	return result
}

// ForEach iterates over all key-value pairs in insertion order
func (jo *JsonObject) ForEach(fn func(key string, value JsonEntity) bool) {
	for _, key := range jo.keys {
		if val, exists := jo.data[key]; exists {
			if !fn(key, val) {
				break
			}
		}
	}
}
