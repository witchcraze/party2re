package http

import (
	"net/http"
	"testing"
)

func TestActionURLResolver(t *testing.T) {
	resolver := NewActionURLResolver()
	charID := "char-999"

	tests := []struct {
		name       string
		actionID   string
		label      string
		category   string
		wantMethod string
		wantURL    string
	}{
		{
			name:       "bank deposit action",
			actionID:   "bank_deposit",
			label:      "預金する",
			category:   "bank",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/bank/deposit",
		},
		{
			name:       "bank withdraw action",
			actionID:   "bank_withdraw",
			label:      "引き出す",
			category:   "bank",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/bank/withdraw",
		},
		{
			name:       "bank state action",
			actionID:   "bank_state",
			label:      "口座情報",
			category:   "bank",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/bank",
		},
		{
			name:       "shop weapon action",
			actionID:   "shop_weapon",
			label:      "武器屋を見る",
			category:   "shop",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/shop/weapon",
		},
		{
			name:       "shop armor action",
			actionID:   "shop_armor",
			label:      "防具屋を見る",
			category:   "shop",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/shop/armor",
		},
		{
			name:       "shop item action",
			actionID:   "shop_item",
			label:      "道具屋を見る",
			category:   "shop",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/shop/item",
		},
		{
			name:       "shop accessory action",
			actionID:   "shop_accessory",
			label:      "装飾品屋を見る",
			category:   "shop",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/shop/accessory",
		},
		{
			name:       "shop batch purchase action",
			actionID:   "shop_batch_purchase",
			label:      "まとめ買い",
			category:   "shop",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/shop/batch-purchase",
		},
		{
			name:       "home sleep action",
			actionID:   "home_sleep",
			label:      "休む",
			category:   "home",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/home/sleep",
		},
		{
			name:       "home wake action",
			actionID:   "home_wake",
			label:      "起きる",
			category:   "home",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/home/wake",
		},
		{
			name:       "chapel bless action",
			actionID:   "chapel_bless",
			label:      "お祈願",
			category:   "chapel",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/chapel/bless",
		},
		{
			name:       "wishingwell exchange action",
			actionID:   "wishingwell_exchange",
			label:      "願いを捧げる",
			category:   "wishingwell",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/wishing-well/exchange",
		},
		{
			name:       "adventure start action",
			actionID:   "adventure_start",
			label:      "冒険に出る",
			category:   "adventure",
			wantMethod: http.MethodPost,
			wantURL:    "/characters/char-999/adventures",
		},
		{
			name:       "fallback for unknown action",
			actionID:   "unknown_custom_action",
			label:      "未知のアクション",
			category:   "custom",
			wantMethod: http.MethodGet,
			wantURL:    "/characters/char-999/actions/unknown_custom_action",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.Resolve(charID, tt.actionID, tt.label, tt.category)

			if got.ID != tt.actionID {
				t.Errorf("expected ID %q, got %q", tt.actionID, got.ID)
			}
			if got.Label != tt.label {
				t.Errorf("expected Label %q, got %q", tt.label, got.Label)
			}
			if got.Category != tt.category {
				t.Errorf("expected Category %q, got %q", tt.category, got.Category)
			}
			if got.Method != tt.wantMethod {
				t.Errorf("expected Method %q, got %q", tt.wantMethod, got.Method)
			}
			if got.URL != tt.wantURL {
				t.Errorf("expected URL %q, got %q", tt.wantURL, got.URL)
			}
		})
	}
}
