package main

import (
	"encoding/json"
	"slices"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
)

func TestProductionSceneRegistryMatchesNavigationSchemas(t *testing.T) {
	pc := newPlayerContext(&coreServices{}, &socServices{}, &econServices{}, &cmbtServices{}, &miscServices{})
	destinations := []string{}
	for _, d := range pc.SceneDefinitions() {
		destinations = append(destinations, d.ID)
		if d.ID == "depot" && (d.Parent != "town" || !d.Pageable || d.CursorPageable || d.SubjectKind != "") {
			t.Fatalf("Depot must use the shared owned facility offset list: %+v", d)
		}
	}
	if !slices.Contains(destinations, "depot") {
		t.Fatal("production Depot scene is missing")
	}
	slices.Sort(destinations)
	var spec map[string]any
	if err := json.Unmarshal(apihttp.OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	checked := 0
	var visit func(any)
	visit = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			if values, ok := node["enum"].([]any); ok && slices.Contains(values, any("town")) && slices.Contains(values, any("bank")) {
				got := make([]string, 0, len(values))
				for _, value := range values {
					got = append(got, value.(string))
				}
				slices.Sort(got)
				if !slices.Equal(got, destinations) {
					t.Errorf("navigation schema excludes registered destinations or invents them: %v; registry=%v", got, destinations)
				}
				checked++
			}
			for _, child := range node {
				visit(child)
			}
		case []any:
			for _, child := range node {
				visit(child)
			}
		}
	}
	visit(spec)
	if checked != 4 {
		t.Fatalf("expected enter/page inputs and destination/page metadata contracts, checked %d", checked)
	}
}
