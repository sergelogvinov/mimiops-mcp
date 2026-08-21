/*
Copyright 2026 Serge Logvinov.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package formatter provides a fallback text formatter for Go structs.
// It converts struct values into a markdown string representation.
package formatter

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// ToMarkdown converts a Go struct value to a markdown string.
// It uses reflection to inspect the struct and format it according to the
// fallback text design specified in docs/fallbackText.md.
func ToMarkdown(v any) string {
	var buf bytes.Buffer

	val := reflect.ValueOf(v)
	if !val.IsValid() {
		return ""
	}

	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return ""
		}
		val = val.Elem()
	}

	// Only structs are supported at the top level
	if val.Kind() != reflect.Struct {
		return ""
	}

	// Collect all printable fields (those with jsonschema tag)
	var fields []fieldInfo

	seen := make(map[string]bool) // Track field names to avoid duplicates
	collectFields(val, val.Type(), &fields, &seen)

	for _, f := range fields {
		formatted := formatValue(f.value, 0)
		if formatted != "" {
			buf.WriteString(f.tag)
			buf.WriteString(": ")
			buf.WriteString(formatted)
			buf.WriteString("\n\n")
		}
	}

	return buf.String()
}

// fieldInfo holds information about a struct field for formatting.
type fieldInfo struct {
	tag       string
	value     reflect.Value
	omitempty bool
}

// formatValue formats a value based on its type.
// depth is used for indentation control in nested structures.
func formatValue(v reflect.Value, depth int) string {
	// Handle nil
	if !v.IsValid() {
		return ""
	}

	// Dereference pointer if needed
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	// Handle interface - unwrap to concrete value
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	switch v.Kind() { //nolint:exhaustive
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Slice, reflect.Array:
		return formatSlice(v, depth)
	case reflect.Struct:
		return formatStruct(v, depth)
	case reflect.Map:
		return formatMap(v, depth)
	default:
		return ""
	}
}

// formatSlice handles slice and array values.
func formatSlice(v reflect.Value, depth int) string {
	if v.Len() == 0 {
		return ""
	}

	// Check if it's a slice of structs
	elemKind := v.Type().Elem().Kind()
	if elemKind == reflect.Struct {
		return formatStructSlice(v, depth)
	}

	// Slice of scalars - join with ", "
	var parts []string
	for i := range v.Len() {
		val := v.Index(i)
		formatted := formatValue(val, depth)
		if formatted != "" {
			parts = append(parts, formatted)
		}
	}

	return strings.Join(parts, ", ")
}

// formatStructSlice renders a slice of structs as a markdown table.
func formatStructSlice(v reflect.Value, depth int) string {
	if v.Len() == 0 {
		return ""
	}

	var buf bytes.Buffer

	// Get the element type
	elemType := v.Type().Elem()

	// Collect printable fields for the table header
	var headerFields []fieldInfo
	seen := make(map[string]bool)
	for field := range elemType.Fields() {
		// Skip anonymous fields in table headers
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("jsonschema")
		if tag == "" {
			continue
		}

		// Check for duplicates
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = true

		// Get default value for the field type
		defaultVal := reflect.Zero(field.Type)

		headerFields = append(headerFields, fieldInfo{
			tag:   tag,
			value: defaultVal,
		})
	}

	if len(headerFields) == 0 {
		return ""
	}

	// Write header row
	buf.WriteString("\n")
	buf.WriteString("|")
	for _, f := range headerFields {
		buf.WriteString(" ")
		buf.WriteString(f.tag)
		buf.WriteString(" |")
	}

	buf.WriteString("\n")
	buf.WriteString("|")
	for range headerFields {
		buf.WriteString(" --- |")
	}

	// Write body rows
	for i := range v.Len() {
		elem := v.Index(i)
		buf.WriteString("\n")
		buf.WriteString("|")

		for _, f := range headerFields {
			fieldVal := elem.FieldByName(f.tag)
			// Try to find the field by name (may differ due to json tag)
			if !fieldVal.IsValid() {
				fieldVal = findFieldByName(elem, f.tag)
			}

			formatted := formatValue(fieldVal, depth+1)
			buf.WriteString(" ")
			buf.WriteString(formatted)
			buf.WriteString(" |")
		}
	}

	buf.WriteString("\n")

	return buf.String()
}

// findFieldByName finds a struct field by its jsonschema tag value.
// It recursively searches through anonymous (embedded) fields.
func findFieldByName(v reflect.Value, tagName string) reflect.Value {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)

		// Recursively search in anonymous fields
		if field.Anonymous {
			fieldVal := v.Field(i)
			if fieldVal.Kind() == reflect.Pointer {
				if fieldVal.IsNil() {
					continue
				}
				fieldVal = fieldVal.Elem()
			}
			if fieldVal.Kind() == reflect.Struct {
				if result := findFieldByName(fieldVal, tagName); result.IsValid() {
					return result
				}
			}
			continue
		}

		tag := field.Tag.Get("jsonschema")
		if tag == tagName {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}

// formatStruct renders a single struct as a nested bullet list.
func formatStruct(v reflect.Value, depth int) string {
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return ""
	}

	var buf bytes.Buffer
	var fields []fieldInfo

	seen := make(map[string]bool)
	collectFields(v, v.Type(), &fields, &seen)

	if len(fields) == 0 {
		return ""
	}

	// Write nested bullet list
	for _, f := range fields {
		formatted := formatValue(f.value, 0)
		if formatted != "" {
			buf.WriteString("\n")
			writeIndent(&buf, depth+1)
			buf.WriteString(f.tag)
			buf.WriteString(": ")
			buf.WriteString(formatted)
			buf.WriteString("\n\n")
		}
	}

	return buf.String()
}

// formatMap renders a map as key=value pairs.
func formatMap(v reflect.Value, depth int) string {
	if v.Len() == 0 {
		return ""
	}

	// Get all keys and sort them for stable output
	keys := v.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})

	var parts []string
	for _, key := range keys {
		val := v.MapIndex(key)
		keyStr := key.String()
		valStr := formatValue(val, depth)
		parts = append(parts, fmt.Sprintf("%s=%s", keyStr, valStr))
	}

	return strings.Join(parts, ", ")
}

// isZero checks if a value is the zero value for its type.
func isZero(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive
	case reflect.String:
		return v.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Array, reflect.Map:
		return v.Len() == 0
	case reflect.Struct:
		// Check if all fields are zero
		for _, field := range v.Fields() {
			if !isZero(field) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// writeIndent writes indentation spaces.
func writeIndent(buf *bytes.Buffer, depth int) {
	for range depth {
		buf.WriteString("    ")
	}
}

// collectFields recursively collects all printable fields from a struct,
// including fields from anonymous (embedded) fields.
func collectFields(val reflect.Value, typ reflect.Type, fields *[]fieldInfo, seen *map[string]bool) {
	for i := range typ.NumField() {
		field := typ.Field(i)

		// Handle anonymous (embedded) fields
		if field.Anonymous {
			fieldVal := val.Field(i)
			if fieldVal.Kind() == reflect.Pointer {
				if fieldVal.IsNil() {
					continue
				}
				fieldVal = fieldVal.Elem()
			}

			if fieldVal.Kind() == reflect.Struct {
				// Recursively collect fields from anonymous field
				collectFields(fieldVal, fieldVal.Type(), fields, seen)
			}
			continue
		}

		tag := field.Tag.Get("jsonschema")
		if tag == "" {
			continue
		}

		// Check for duplicate field names (anonymous fields can cause duplicates)
		if _, exists := (*seen)[tag]; exists {
			continue
		}
		(*seen)[tag] = true

		fieldVal := val.Field(i)
		if fieldVal.Kind() == reflect.Pointer && fieldVal.IsNil() {
			continue
		}

		omitempty := strings.Contains(field.Tag.Get("json"), "omitempty")
		if omitempty && isZero(fieldVal) {
			continue
		}

		*fields = append(*fields, fieldInfo{
			tag:       tag,
			value:     fieldVal,
			omitempty: omitempty,
		})
	}
}
