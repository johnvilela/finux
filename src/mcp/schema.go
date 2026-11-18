package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// argument describes one field of an arguments struct.
type argument struct {
	name, description string
	index             int
	required          bool
	enum              []string
	// kind is the JSON Schema type; label is that type in an error.
	kind, label string
}

// arguments reads the fields of an arguments struct. The json tag gives
// the name; a field without omitempty is required. The mcp tag gives the
// description and the enum tag the values a string accepts.
func arguments(of reflect.Type) []argument {
	var list []argument
	for i := range of.NumField() {
		field := of.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		current := argument{name: name, index: i, description: field.Tag.Get("mcp"), required: !strings.Contains(options, "omitempty")}
		if enum := field.Tag.Get("enum"); enum != "" {
			current.enum = strings.Split(enum, ",")
		}
		kind := field.Type
		if kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
		switch kind.Kind() {
		case reflect.String:
			current.kind, current.label = "string", "a string"
		case reflect.Bool:
			current.kind, current.label = "boolean", "a boolean"
		case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64:
			current.kind, current.label = "integer", "an integer"
		case reflect.Slice:
			current.kind, current.label = "array", "a list of strings"
		default:
			panic(fmt.Sprintf("mcp: argument %s has the unsupported type %s", name, field.Type))
		}
		list = append(list, current)
	}
	return list
}

// schema returns the JSON Schema of an arguments struct.
func schema(list []argument) map[string]any {
	properties := map[string]any{}
	required := []string{}
	for _, current := range list {
		property := map[string]any{"type": current.kind}
		if current.description != "" {
			property["description"] = current.description
		}
		if current.enum != nil {
			property["enum"] = current.enum
		}
		if current.kind == "array" {
			property["items"] = map[string]any{"type": "string"}
		}
		properties[current.name] = property
		if current.required {
			required = append(required, current.name)
		}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

// decode reads the arguments of a call into the struct target points to.
// Its errors tell the agent which argument to correct.
func decode(raw json.RawMessage, list []argument, target reflect.Value) error {
	if raw = bytes.TrimSpace(raw); len(raw) == 0 || string(raw) == "null" {
		raw = json.RawMessage("{}")
	}
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return errors.New("arguments must be an object")
	}
	names := make([]string, 0, len(given))
	for name := range given {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !slices.ContainsFunc(list, func(current argument) bool { return current.name == name }) {
			return fmt.Errorf("unknown argument %q", name)
		}
	}
	for _, current := range list {
		value, ok := given[current.name]
		if !ok || string(bytes.TrimSpace(value)) == "null" {
			if current.required {
				return fmt.Errorf("missing argument %q", current.name)
			}
			continue
		}
		field := target.Field(current.index)
		if err := json.Unmarshal(value, field.Addr().Interface()); err != nil {
			return fmt.Errorf("argument %q must be %s", current.name, current.label)
		}
		if current.enum != nil {
			text := field
			if text.Kind() == reflect.Pointer {
				text = text.Elem()
			}
			// An agent can send BRL as brl; the tool sets the letter case.
			if !slices.ContainsFunc(current.enum, func(value string) bool { return strings.EqualFold(value, text.String()) }) {
				return fmt.Errorf("argument %q must be one of %s", current.name, strings.Join(current.enum, ", "))
			}
		}
	}
	return nil
}

// New makes a tool from a function. A is a struct that holds the
// arguments of the tool; its tags give the schema.
func New[A any](name, description string, call func(A) (any, error)) Tool {
	list := arguments(reflect.TypeFor[A]())
	return Tool{
		Name: name, Description: description, InputSchema: schema(list),
		Call: func(raw json.RawMessage) (any, error) {
			var values A
			if err := decode(raw, list, reflect.ValueOf(&values).Elem()); err != nil {
				return nil, err
			}
			return call(values)
		},
	}
}
