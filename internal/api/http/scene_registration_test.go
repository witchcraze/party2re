package http

import (
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/playercontext"
)

// A location-bound Gateway command no facility registers is admitted nowhere.
// Registering the facilities also validates each listed command against the catalog.
func TestEveryLocationBoundGatewayCommandBelongsToAFacility(t *testing.T) {
	definitions := []playercontext.SceneDefinition{}
	for _, id := range FacilityIDs() {
		d := playercontext.SceneDefinition{ID: id, Parent: "town"}
		if id == "town" {
			d.Parent = ""
		}
		definitions = append(definitions, d)
	}
	service := playercontext.NewService(nil, nil, nil, playercontext.WithNavigation(&gatewayNavigationStore{}, definitions...))
	(&Handler{}).registerScenes(service)
	admitted := []string{}
	for _, d := range service.SceneDefinitions() {
		admitted = append(admitted, d.Actions...)
		admitted = append(admitted, d.SubjectActions...)
		admitted = append(admitted, d.OwnedSubjectActions...)
	}
	for _, def := range playercontext.DefaultCatalog {
		if def.OperationID != "executeCharacterAction" || !def.RequiredGates.Has(playercontext.GateLocationCheck) || def.ActivityKind != "" || def.Category == "navigation" || def.ID == "home_wake" {
			continue
		}
		if !slices.Contains(admitted, def.ID) {
			t.Errorf("%s is admitted by no facility", def.ID)
		}
	}
}
