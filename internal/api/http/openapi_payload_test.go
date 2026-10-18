package http_test

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Check the payload shapes used by the contract fixtures, including exact integer bounds.
func checkOpenAPIPayload(schemas map[string]any, schema map[string]any, value any) error {
	if ref, ok := schema["$ref"].(string); ok {
		return checkOpenAPIPayload(schemas, schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any), value)
	}
	if alternatives, ok := schema["anyOf"].([]any); ok {
		for _, alternative := range alternatives {
			combined := make(map[string]any)
			for key, constraint := range schema {
				if key != "anyOf" {
					combined[key] = constraint
				}
			}
			for key, constraint := range alternative.(map[string]any) {
				combined[key] = constraint
			}
			if checkOpenAPIPayload(schemas, combined, value) == nil {
				return nil
			}
		}
		return fmt.Errorf("value matches no anyOf alternative")
	}
	kind := "null"
	switch value.(type) {
	case map[string]any:
		kind = "object"
	case []any:
		kind = "array"
	case string:
		kind = "string"
	case bool:
		kind = "boolean"
	case json.Number:
		kind = "number"
		if n, ok := new(big.Rat).SetString(string(value.(json.Number))); ok && n.IsInt() {
			kind = "integer"
		}
	}
	allowed := schema["type"] == nil || schema["type"] == kind || (schema["type"] == "number" && kind == "integer")
	if types, ok := schema["type"].([]any); ok {
		for _, candidate := range types {
			allowed = allowed || candidate == kind
		}
	}
	if !allowed {
		return fmt.Errorf("got %s, want %v", kind, schema["type"])
	}
	if choices, ok := schema["enum"].([]any); ok {
		found := false
		for _, choice := range choices {
			found = found || reflect.DeepEqual(choice, value)
		}
		if !found {
			return fmt.Errorf("value %v outside enum", value)
		}
	}
	switch value := value.(type) {
	case string:
		if minimum, ok := schema["minLength"].(json.Number); ok {
			length, err := minimum.Int64()
			if err != nil {
				return err
			}
			if int64(utf8.RuneCountInString(value)) < length {
				return fmt.Errorf("string violates minLength")
			}
		}
		if pattern, ok := schema["pattern"].(string); ok {
			matched, err := regexp.MatchString(pattern, value)
			if err != nil {
				return err
			}
			if !matched {
				return fmt.Errorf("string violates pattern")
			}
		}
	case map[string]any:
		if required, ok := schema["required"].([]any); ok {
			for _, key := range required {
				if _, ok := value[key.(string)]; !ok {
					return fmt.Errorf("missing required property %s", key)
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		for key, child := range value {
			childSchema, ok := properties[key].(map[string]any)
			if !ok {
				if schema["additionalProperties"] == true {
					continue
				}
				childSchema, ok = schema["additionalProperties"].(map[string]any)
				if !ok {
					return fmt.Errorf("undocumented property %s", key)
				}
			}
			if err := checkOpenAPIPayload(schemas, childSchema, child); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case []any:
		for _, bound := range []string{"minItems", "maxItems"} {
			if n, ok := schema[bound].(json.Number); ok {
				limit, err := n.Int64()
				if err != nil {
					return err
				}
				if bound == "minItems" && int64(len(value)) < limit || bound == "maxItems" && int64(len(value)) > limit {
					return fmt.Errorf("array violates %s", bound)
				}
			}
		}
		for i, child := range value {
			if err := checkOpenAPIPayload(schemas, schema["items"].(map[string]any), child); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
	case json.Number:
		n, ok := new(big.Rat).SetString(string(value))
		if !ok {
			return fmt.Errorf("invalid number %s", value)
		}
		for _, bound := range []string{"minimum", "maximum"} {
			if raw, ok := schema[bound].(json.Number); ok {
				limit, ok := new(big.Rat).SetString(string(raw))
				if !ok {
					return fmt.Errorf("invalid bound %s", raw)
				}
				if bound == "minimum" && n.Cmp(limit) < 0 || bound == "maximum" && n.Cmp(limit) > 0 {
					return fmt.Errorf("number violates %s", bound)
				}
			}
		}
	}
	return nil
}
