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
	PathPattern string // e.g. "/api/v1/characters/%s/actions"
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
		"alchemy_synthesize":    {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"alchemy_recipes":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"fleamarket_list":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_build":           {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_list_gold":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_list_barter":     {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_withdraw":        {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_buy":             {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_trade":           {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_rename":          {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_wallpaper":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_interior_add":    {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_interior_rename": {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"store_interior_clean":  {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"fleamarket_purchase":   {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"fleamarket_cancel":     {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blackmarket_sacrifice": {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blackmarket_trade":     {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_buy":          {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"altar_pray":            {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"altar_wish":            {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"wishingwell_exchange":  {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_sell":         {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_send":         {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_synthesize":   {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_appraise":     {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"gemstore_sort":         {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blacksmith_seal":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blacksmith_name":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blacksmith_deposit":    {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"blacksmith_withdraw":   {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"auction_send":          {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"depot_send_item": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_send_money":    {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"secretshop_puffpuff": {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"depot_deposit": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_withdraw": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_sort": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_sell": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_sell_batch": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"depot_expand": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"bank_deposit": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"bank_withdraw": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"secretshop_purchase": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"shop_purchase":       {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"shop_sell":           {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"shop_batch_purchase": {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"shop_synthesize":     {Method: http.MethodPost, PathPattern: "/api/v1/characters/%s/actions"},
		"home_sleep": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/home/sleep",
		},
		"home_wake": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/home/wake",
		},
		"chapel_pray": {
			Method:      http.MethodPost,
			PathPattern: "/api/v1/characters/%s/actions",
		},
		"adventure_start": {
			Method:      http.MethodPost,
			PathPattern: "/characters/%s/adventures",
		},
	}
}
