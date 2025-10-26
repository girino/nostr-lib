package json

import (
	"fmt"
)

func Example() {
	// Create a new ordered JSON object
	obj := NewJsonObject()

	// Add some fields in a specific order
	obj.Set("name", NewJsonValue("John"))
	obj.Set("age", NewJsonValue(30))
	obj.Set("active", NewJsonValue(true))

	// Marshal to JSON
	data, err := obj.MarshalJSON()
	if err != nil {
		panic(err)
	}

	fmt.Println(string(data))

	// Unmarshal JSON back into our structure
	parsed, err := Unmarshal(data)
	if err != nil {
		panic(err)
	}

	// Type assert and access
	if parsedObj, ok := parsed.(*JsonObject); ok {
		fmt.Printf("Has 'name': %v\n", parsedObj.Has("name"))
		keys := parsedObj.Keys()
		fmt.Printf("Keys: %v\n", keys)
	}

	// Output:
	// {"name":"John","age":30,"active":true}
	// Has 'name': true
	// Keys: [name age active]
}

func ExampleJsonValue() {
	// Create values of different types
	v1 := NewJsonValue(42)
	v2 := NewJsonValue(3.14)
	v3 := NewJsonValue("hello")

	fmt.Printf("Int: %d\n", v1.ToInterface())
	fmt.Printf("Float: %f\n", v2.ToInterface())
	fmt.Printf("String: %s\n", v3.ToInterface())

	// Output:
	// Int: 42
	// Float: 3.140000
	// String: hello
}

func ExampleJsonList() {
	list := NewJsonList()

	list.Append(NewJsonValue(1))
	list.Append(NewJsonValue(2))
	list.Append(NewJsonValue(3))

	fmt.Printf("Length: %d\n", list.Length())

	data, err := Marshal(list)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(data))

	// Output:
	// Length: 3
	// [1,2,3]
}
