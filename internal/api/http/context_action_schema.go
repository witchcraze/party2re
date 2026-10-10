package http

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/witchcraze/party2re/internal/core/character"
)

// Keep discovery tied to the same ActionID-specific contract as execution and
// the catalog drift gate. Raw fields preserve signed int64 bounds exactly.
var contextParamsSchemas = loadContextParamsSchemas()

func loadContextParamsSchemas() map[string]json.RawMessage {
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				AllOf []struct {
					If struct {
						Properties map[string]struct{ Const json.RawMessage }
					}
					Then struct{ Properties map[string]json.RawMessage }
				}
			}
		}
	}
	if err := json.Unmarshal(openapiJSON, &spec); err != nil {
		panic(err)
	}
	schemas := make(map[string]json.RawMessage)
	for _, condition := range spec.Components.Schemas["CharacterActionRequest"].AllOf {
		var id string
		if err := json.Unmarshal(condition.If.Properties["action"].Const, &id); err != nil {
			panic(err)
		}
		if _, exists := schemas[id]; exists {
			panic("duplicate command schema: " + id)
		}
		schemas[id] = condition.Then.Properties["params"]
	}
	return schemas
}

func (h *Handler) contextAction(id string, template map[string]any, actor character.Character) (ContextAction, error) {
	def, ok := actionDefinition(id)
	if !ok {
		return ContextAction{}, fmt.Errorf("unknown offered command: %s", id)
	}
	raw, ok := contextParamsSchemas[id]
	if !ok {
		return ContextAction{}, fmt.Errorf("missing offered command schema: %s", id)
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil {
		return ContextAction{}, err
	}
	var properties map[string]map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &properties); err != nil {
		return ContextAction{}, err
	}
	if id == "scene_enter" {
		values := make([]string, 0, len(h.sceneAdapters))
		for destination, adapter := range h.sceneAdapters {
			if adapter.read != nil && (adapter.definition.CanEnter == nil || adapter.definition.CanEnter(actor)) {
				values = append(values, destination)
			}
		}
		slices.Sort(values)
		enum, err := json.Marshal(values)
		if err != nil {
			return ContextAction{}, err
		}
		properties["destination"]["enum"] = enum
	}
	for name, value := range template {
		property, ok := properties[name]
		if !ok {
			return ContextAction{}, fmt.Errorf("unknown template parameter: %s.%s", id, name)
		}
		constant, err := json.Marshal(value)
		if err != nil {
			return ContextAction{}, err
		}
		property["const"] = constant
	}
	var err error
	schema["properties"], err = json.Marshal(properties)
	if err != nil {
		return ContextAction{}, err
	}
	// Empty-object schemas still carry non-null required arrays.
	schema["required"], err = json.Marshal(append([]string{}, def.RequiredParams...))
	if err != nil {
		return ContextAction{}, err
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return ContextAction{}, err
	}
	style := "secondary"
	if def.Category == "adventure" {
		style = "primary"
	}
	return ContextAction{Action: id, Label: def.Label, Category: def.Category, Style: style,
		RequiredParams: append([]string{}, def.RequiredParams...), ParamsSchema: encoded, ParamsTemplate: template}, nil
}
