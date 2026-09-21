package http

import (
	"fmt"
	"net/http"
	"strings"
)

// ResolvedAction represents a complete, client-ready actionable link with HTTP routing metadata.
// It bridges domain-level ActionIDs into HATEOAS-style discoverable endpoint links.
type ResolvedAction struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Method   string `json:"method"`
	URL      string `json:"url"`
}

// ActionEndpoint defines the HTTP method and path pattern for an action.
type ActionEndpoint struct {
	Method      string
	PathPattern string // e.g. "/characters/%s/bank/deposit"
}

// ActionURLResolver translates domain-level ActionIDs into concrete HTTP endpoints.
// This decouples the core domain layer (internal/playercontext) from HTTP routing knowledge.
type ActionURLResolver struct {
	endpoints map[string]ActionEndpoint
}

// NewActionURLResolver initializes an ActionURLResolver with standard application routes.
func NewActionURLResolver() *ActionURLResolver {
	return &ActionURLResolver{
		endpoints: defaultActionEndpoints(),
	}
}

// Resolve maps a character ID and domain action metadata to a client-facing ResolvedAction.
func (r *ActionURLResolver) Resolve(characterID, actionID, label, category string) ResolvedAction {
	ep, ok := r.endpoints[actionID]
	if !ok {
		return ResolvedAction{
			ID:       actionID,
			Label:    label,
			Category: category,
			Method:   http.MethodGet,
			URL:      fmt.Sprintf("/characters/%s/actions/%s", characterID, actionID),
		}
	}

	url := ep.PathPattern
	if strings.Contains(url, "%s") {
		url = fmt.Sprintf(url, characterID)
	}

	return ResolvedAction{
		ID:       actionID,
		Label:    label,
		Category: category,
		Method:   ep.Method,
		URL:      url,
	}
}

// defaultActionEndpoints registers the canonical endpoints for Party2 character actions.
func defaultActionEndpoints() map[string]ActionEndpoint {
	return map[string]ActionEndpoint{
		"bank_state": {
			Method:      http.MethodGet,
			PathPattern: "/characters/%s/bank",
		},
		"bank_deposit": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/bank/deposit",
		},
		"bank_withdraw": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/bank/withdraw",
		},
		"shop_weapon": {
			Method:      http.MethodGet,
			PathPattern: "/characters/%s/shop/weapon",
		},
		"shop_armor": {
			Method:      http.MethodGet,
			PathPattern: "/characters/%s/shop/armor",
		},
		"shop_item": {
			Method:      http.MethodGet,
			PathPattern: "/characters/%s/shop/item",
		},
		"shop_batch_purchase": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/shop/batch-purchase",
		},
		"home_sleep": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/home/sleep",
		},
		"home_wake": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/home/wake",
		},
		"chapel_bless": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/chapel/bless",
		},
		"wishingwell_exchange": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/wishing-well/exchange",
		},
		"adventure_start": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/adventures",
		},
	}
}
