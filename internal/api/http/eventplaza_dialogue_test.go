package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/eventplaza"
)

func TestEventPlazaPurchaseDialogue(t *testing.T) {
	for _, tc := range []struct {
		name    string
		depot   bool
		message string
	}{
		{"inventory", false, "はい、魔法の粉です"},
		{"depot", true, "魔法の粉はHeroさんの預かり所に送っておきましたよ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := eventplaza.BazaarPurchaseResult{
				CharacterID: "char-1", Item: eventplaza.BazaarItem{Name: "魔法の粉"},
				Quantity: 1, TotalPrice: 6000, RemainingGold: 44000,
				InventoryInstanceID: "instance-1", TransferredToDepot: tc.depot,
			}
			svc := &mockEventPlazaService{
				purchaseBazaarItemFn: func(context.Context, string, string, int) (eventplaza.BazaarPurchaseResult, error) {
					return result, nil
				},
			}
			handler, err := createTestHandler(apihttp.WithEventPlaza(svc))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/eventplaza/merchant/purchase", strings.NewReader(`{"character_id":"char-1","item_id":"item-081","quantity":1}`))
			req.Header.Set("Authorization", "Bearer valid-session")
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler.Router().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var response struct {
				eventplaza.BazaarPurchaseResult
				NPCMessage string `json:"npc_message"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
			if response.NPCMessage != tc.message {
				t.Errorf("npc_message = %q; want %q", response.NPCMessage, tc.message)
			}
			if response.BazaarPurchaseResult != result {
				t.Errorf("purchase facts = %+v; want %+v", response.BazaarPurchaseResult, result)
			}
		})
	}
}
