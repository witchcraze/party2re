package main

import (
	"net/http"
	"testing"
)

func TestRankingProductionHTTPWiring(t *testing.T) {
	app := newScenarioApp(t)
	app.call("", http.MethodGet, "/legends/comp_mon", "", http.StatusOK, nil)
}
