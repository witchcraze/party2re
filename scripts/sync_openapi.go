package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Route represents an HTTP route registration extracted from Handler.Router().
type Route struct {
	Method string
	Path   string
}

func main() {
	checkOnly := flag.Bool("check", false, "Only check if OpenAPI specs are synchronized and all routes are documented without modifying files")
	scaffoldOnly := flag.Bool("scaffold", false, "Explicitly scaffold missing routes from internal/api/http/handler.go into docs/api/paths/*.json")
	flag.Parse()

	rootDir, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	basePath := filepath.Join(rootDir, "docs", "api", "base.json")
	pathsDir := filepath.Join(rootDir, "docs", "api", "paths")
	docsPath := filepath.Join(rootDir, "docs", "api", "openapi.json")
	pkgPath := filepath.Join(rootDir, "internal", "api", "http", "openapi.json")
	handlerPath := filepath.Join(rootDir, "internal", "api", "http", "handler.go")

	if _, err := os.Stat(basePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: base spec %s not found: %v\n", basePath, err)
		os.Exit(1)
	}

	if _, err := os.Stat(pathsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: paths directory %s not found: %v\n", pathsDir, err)
		os.Exit(1)
	}

	handlerRoutes, err := extractRegisteredRoutes(handlerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error extracting routes from %s: %v\n", handlerPath, err)
		os.Exit(1)
	}

	// 1. If in --check mode, verify route coverage and file synchronization
	if *checkOnly {
		if err := runCheck(basePath, pathsDir, docsPath, pkgPath, handlerRoutes); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 2. Otherwise (sync or scaffold mode):
	// Check for missing routes in handler.go and scaffold them into docs/api/paths/{module}.json
	scaffoldedCount, err := scaffoldMissingRoutes(pathsDir, handlerRoutes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scaffolding missing routes: %v\n", err)
		os.Exit(1)
	}
	if scaffoldedCount > 0 {
		fmt.Printf("Scaffolded %d new endpoint(s) into docs/api/paths/\n", scaffoldedCount)
	}

	// 3. Bundle base.json + docs/api/paths/*.json -> docs/api/openapi.json & internal/api/http/openapi.json
	bundledData, pathCount, routeCount, err := bundleOpenAPISpec(basePath, pathsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error bundling OpenAPI specification: %v\n", err)
		os.Exit(1)
	}

	// Verify route coverage against handler.go
	missing := checkRouteCoverage(bundledData, handlerRoutes)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d route(s) still missing from OpenAPI spec after bundling:\n", len(missing))
		for _, r := range missing {
			fmt.Fprintf(os.Stderr, "  - %s %s\n", r.Method, r.Path)
		}
	}

	// Format and rewrite all docs/api/paths/*.json files cleanly
	if err := formatPathFiles(pathsDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting path files: %v\n", err)
		os.Exit(1)
	}

	// Write to both docs/api/openapi.json and internal/api/http/openapi.json
	if err := os.WriteFile(docsPath, bundledData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", docsPath, err)
		os.Exit(1)
	}

	if err := os.WriteFile(pkgPath, bundledData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", pkgPath, err)
		os.Exit(1)
	}

	actionDesc := "synchronized and formatted"
	if *scaffoldOnly {
		actionDesc = "scaffolded and synchronized"
	}
	fmt.Printf("Successfully %s OpenAPI 3.1 specification (%d paths, %d operations)\n", actionDesc, pathCount, routeCount)
	fmt.Printf("  - %s\n", docsPath)
	fmt.Printf("  - %s\n", pkgPath)
}

func runCheck(basePath, pathsDir, docsPath, pkgPath string, handlerRoutes []Route) error {
	bundledData, pathCount, routeCount, err := bundleOpenAPISpec(basePath, pathsDir)
	if err != nil {
		return fmt.Errorf("failed to bundle OpenAPI spec: %w", err)
	}

	missing := checkRouteCoverage(bundledData, handlerRoutes)
	if len(missing) > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("%d route(s) registered in handler.go are missing from OpenAPI specification:\n", len(missing)))
		for _, r := range missing {
			b.WriteString(fmt.Sprintf("  - %s %s\n", r.Method, r.Path))
		}
		b.WriteString("Run 'make openapi-sync' to scaffold missing routes and synchronize.\n")
		return fmt.Errorf("%s", b.String())
	}

	docsCurrent, docsErr := os.ReadFile(docsPath)
	pkgCurrent, pkgErr := os.ReadFile(pkgPath)

	if docsErr != nil || pkgErr != nil {
		return fmt.Errorf("OpenAPI target files missing (docs: %v, pkg: %v). Run 'make openapi-sync'", docsErr, pkgErr)
	}

	if !bytes.Equal(bytes.TrimSpace(docsCurrent), bytes.TrimSpace(bundledData)) ||
		!bytes.Equal(bytes.TrimSpace(pkgCurrent), bytes.TrimSpace(bundledData)) {
		return fmt.Errorf("OpenAPI specs are out of sync or unformatted. Run 'make openapi-sync' to synchronize")
	}

	fmt.Printf("OpenAPI specs are fully synchronized, formatted, and cover 100%% of registered routes (%d paths, %d operations).\n", pathCount, routeCount)
	return nil
}

// bundleOpenAPISpec loads docs/api/base.json and merges all docs/api/paths/*.json files into the paths field.
func bundleOpenAPISpec(basePath, pathsDir string) ([]byte, int, int, error) {
	baseData, err := os.ReadFile(basePath)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read %s: %w", basePath, err)
	}

	var specMap map[string]interface{}
	if err := json.Unmarshal(baseData, &specMap); err != nil {
		return nil, 0, 0, fmt.Errorf("invalid base JSON in %s: %w", basePath, err)
	}

	entries, err := os.ReadDir(pathsDir)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to read paths directory %s: %w", pathsDir, err)
	}

	mergedPaths := make(map[string]interface{})
	pathSourceFile := make(map[string]string)
	totalOperations := 0

	// Sort filenames to guarantee deterministic loading
	var jsonFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles = append(jsonFiles, entry.Name())
		}
	}
	sort.Strings(jsonFiles)

	for _, filename := range jsonFiles {
		filePath := filepath.Join(pathsDir, filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		var pathItems map[string]interface{}
		if err := json.Unmarshal(data, &pathItems); err != nil {
			return nil, 0, 0, fmt.Errorf("invalid JSON in %s: %w", filePath, err)
		}

		for pathKey, item := range pathItems {
			if prevFile, exists := pathSourceFile[pathKey]; exists {
				return nil, 0, 0, fmt.Errorf("duplicate path %q defined in both %s and %s", pathKey, prevFile, filename)
			}
			pathSourceFile[pathKey] = filename
			mergedPaths[pathKey] = item

			if opMap, ok := item.(map[string]interface{}); ok {
				for k, v := range opMap {
					kLower := strings.ToLower(k)
					if kLower == "get" || kLower == "post" || kLower == "put" || kLower == "delete" || kLower == "patch" {
						if _, isOp := v.(map[string]interface{}); isOp {
							totalOperations++
						}
					}
				}
			}
		}
	}

	specMap["paths"] = mergedPaths

	formatted, err := json.MarshalIndent(specMap, "", "  ")
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to format spec JSON: %w", err)
	}
	formatted = append(formatted, '\n')

	return formatted, len(mergedPaths), totalOperations, nil
}

// scaffoldMissingRoutes checks each route in handlerRoutes against docs/api/paths/*.json,
// and scaffolds any missing endpoints.
func scaffoldMissingRoutes(pathsDir string, handlerRoutes []Route) (int, error) {
	// 1. Read existing paths into memory by module file
	entries, err := os.ReadDir(pathsDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", pathsDir, err)
	}

	modulePaths := make(map[string]map[string]map[string]interface{})
	existingRoutes := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			modName := strings.TrimSuffix(entry.Name(), ".json")
			filePath := filepath.Join(pathsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				return 0, err
			}
			var items map[string]map[string]interface{}
			if err := json.Unmarshal(data, &items); err != nil {
				return 0, fmt.Errorf("invalid JSON in %s: %w", filePath, err)
			}
			modulePaths[modName] = items

			for pathKey, methodMap := range items {
				for method := range methodMap {
					existingRoutes[strings.ToUpper(method)+" "+pathKey] = true
				}
			}
		}
	}

	scaffoldedCount := 0
	modifiedModules := make(map[string]bool)

	for _, r := range handlerRoutes {
		routeKey := r.Method + " " + r.Path
		if existingRoutes[routeKey] {
			continue
		}

		// Determine target module
		targetMod := moduleForRoute(r)
		if modulePaths[targetMod] == nil {
			modulePaths[targetMod] = make(map[string]map[string]interface{})
		}
		if modulePaths[targetMod][r.Path] == nil {
			modulePaths[targetMod][r.Path] = make(map[string]interface{})
		}

		// Generate operation boilerplate
		op := generateBoilerplateOperation(r, targetMod)
		modulePaths[targetMod][r.Path][strings.ToLower(r.Method)] = op

		existingRoutes[routeKey] = true
		modifiedModules[targetMod] = true
		scaffoldedCount++
	}

	// Write back modified module files
	for mod := range modifiedModules {
		filePath := filepath.Join(pathsDir, mod+".json")
		data, err := json.MarshalIndent(modulePaths[mod], "", "  ")
		if err != nil {
			return 0, err
		}
		data = append(data, '\n')
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return 0, err
		}
	}

	return scaffoldedCount, nil
}

func formatPathFiles(pathsDir string) error {
	entries, err := os.ReadDir(pathsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			filePath := filepath.Join(pathsDir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				return err
			}
			var obj map[string]interface{}
			if err := json.Unmarshal(data, &obj); err != nil {
				return err
			}
			formatted, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return err
			}
			formatted = append(formatted, '\n')
			if err := os.WriteFile(filePath, formatted, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkRouteCoverage(bundledData []byte, handlerRoutes []Route) []Route {
	var spec struct {
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(bundledData, &spec); err != nil {
		return handlerRoutes
	}

	var missing []Route
	for _, r := range handlerRoutes {
		pathItem, exists := spec.Paths[r.Path]
		if !exists {
			missing = append(missing, r)
			continue
		}
		methodKey := strings.ToLower(r.Method)
		if _, hasMethod := pathItem[methodKey]; !hasMethod {
			missing = append(missing, r)
		}
	}
	return missing
}

func extractRegisteredRoutes(handlerFilePath string) ([]Route, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, handlerFilePath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", handlerFilePath, err)
	}

	var routes []Route
	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "Router" {
			return true
		}

		ast.Inspect(fn.Body, func(bn ast.Node) bool {
			call, ok := bn.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}

			if len(call.Args) >= 1 {
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				pattern := strings.Trim(lit.Value, "\"")
				parts := strings.SplitN(pattern, " ", 2)
				if len(parts) == 2 {
					routes = append(routes, Route{
						Method: parts[0],
						Path:   parts[1],
					})
				}
			}
			return true
		})
		return false
	})

	return routes, nil
}

var pathParamRegex = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

func generateBoilerplateOperation(route Route, module string) map[string]interface{} {
	op := make(map[string]interface{})
	op["summary"] = fmt.Sprintf("%s %s endpoint", route.Method, route.Path)
	op["operationId"] = generateOperationID(route.Method, route.Path)
	op["tags"] = []string{moduleToTagName(module)}

	// Parameters
	matches := pathParamRegex.FindAllStringSubmatch(route.Path, -1)
	if len(matches) > 0 {
		var params []map[string]interface{}
		for _, m := range matches {
			paramName := m[1]
			params = append(params, map[string]interface{}{
				"name":        paramName,
				"in":          "path",
				"required":    true,
				"description": fmt.Sprintf("%s parameter", paramName),
				"schema": map[string]interface{}{
					"type": "string",
				},
			})
		}
		op["parameters"] = params
	}

	// Standard responses
	op["responses"] = map[string]interface{}{
		"200": map[string]interface{}{
			"description": "Successful operation",
		},
		"400": map[string]interface{}{
			"description": "Bad request",
		},
		"401": map[string]interface{}{
			"description": "Unauthorized",
		},
		"403": map[string]interface{}{
			"description": "Forbidden",
		},
		"500": map[string]interface{}{
			"description": "Internal server error",
		},
	}

	return op
}

func generateOperationID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	clean := pathParamRegex.ReplaceAllString(path, "$1")
	parts := strings.FieldsFunc(clean, func(r rune) bool {
		return r == '/' || r == '-' || r == '_'
	})
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

func moduleForRoute(route Route) string {
	p := route.Path
	switch {
	case strings.HasPrefix(p, "/admin"):
		return "admin"
	case strings.HasPrefix(p, "/adventures") || p == "/characters/{id}/adventures" || p == "/characters/{id}/adventure-chronicle":
		return "adventure"
	case strings.HasPrefix(p, "/auctions"):
		return "auction"
	case p == "/players" || p == "/sessions":
		return "auth"
	case strings.HasPrefix(p, "/players/"):
		return "player"
	case strings.HasPrefix(p, "/characters/{id}/blackmarket"):
		return "blackmarket"
	case strings.HasPrefix(p, "/characters/{id}/bosses"):
		return "boss"
	case strings.HasPrefix(p, "/characters/{id}/casino"):
		return "casino"
	case strings.HasPrefix(p, "/challenges") || strings.HasPrefix(p, "/characters/{id}/challenges"):
		return "challenge"
	case strings.HasPrefix(p, "/characters/{id}/chapel"):
		return "chapel"
	case strings.HasPrefix(p, "/characters/{id}/collections"):
		return "collection"
	case strings.HasPrefix(p, "/characters/{id}/contest") || strings.HasPrefix(p, "/characters/{id}/photos") || strings.HasPrefix(p, "/contest"):
		return "contest"
	case strings.HasPrefix(p, "/characters/{id}/custom-skills"):
		return "custom_skill"
	case strings.HasPrefix(p, "/characters/{id}/delivery"):
		return "delivery"
	case strings.HasPrefix(p, "/characters/{id}/dungeons"):
		return "dungeon"
	case strings.HasPrefix(p, "/eventplaza"):
		return "eventplaza"
	case strings.HasPrefix(p, "/characters/{id}/farm"):
		return "farm"
	case strings.HasPrefix(p, "/characters/{id}/fleamarket") || strings.HasPrefix(p, "/fleamarket"):
		return "fleamarket"
	case strings.HasPrefix(p, "/characters/{id}/gemstore") || strings.HasPrefix(p, "/gemstore"):
		return "gemstore"
	case strings.HasPrefix(p, "/characters/{id}/god") || strings.HasPrefix(p, "/god"):
		return "god"
	case strings.HasPrefix(p, "/helpers"):
		return "helper"
	case strings.HasPrefix(p, "/homes"):
		return "home"
	case strings.HasPrefix(p, "/letters"):
		return "letter"
	case strings.HasPrefix(p, "/characters/{id}/lottery"):
		return "lottery"
	case strings.HasPrefix(p, "/characters/{id}/achievements") || strings.HasPrefix(p, "/characters/{id}/medals") || strings.HasPrefix(p, "/medals"):
		return "medal"
	case strings.HasPrefix(p, "/characters/{id}/monsters") || strings.HasPrefix(p, "/monster"):
		return "monster"
	case strings.HasPrefix(p, "/news"):
		return "news"
	case strings.HasPrefix(p, "/notifications"):
		return "notification"
	case strings.HasPrefix(p, "/park"):
		return "park"
	case strings.HasPrefix(p, "/parties"):
		return "party"
	case strings.HasPrefix(p, "/characters/{id}/pvp"):
		return "pvp"
	case strings.HasPrefix(p, "/rankings"):
		return "ranking"
	case strings.HasPrefix(p, "/rescues"):
		return "rescue"
	case strings.HasPrefix(p, "/characters/{id}/secretshop"):
		return "secretshop"
	case strings.HasPrefix(p, "/shop"):
		return "shop"
	case strings.HasPrefix(p, "/characters/{id}/tavern") || strings.HasPrefix(p, "/tavern"):
		return "tavern"
	case p == "/health" || p == "/maintenance" || p == "/openapi.json":
		return "system"
	case strings.HasPrefix(p, "/characters") || p == "/jobs" || strings.HasPrefix(p, "/naming-hall"):
		return "character"
	default:
		trimmed := strings.TrimPrefix(p, "/")
		seg := strings.Split(trimmed, "/")[0]
		seg = strings.ReplaceAll(seg, "{", "")
		seg = strings.ReplaceAll(seg, "}", "")
		seg = strings.ReplaceAll(seg, "-", "_")
		if seg == "" {
			seg = "misc"
		}
		return seg
	}
}

func moduleToTagName(module string) string {
	tagMap := map[string]string{
		"admin":        "Admin",
		"adventure":    "Adventure",
		"auction":      "Auctions",
		"auth":         "Auth",
		"blackmarket":  "Black Market",
		"boss":         "Combat",
		"casino":       "Casino",
		"challenge":    "Combat",
		"chapel":       "Chapel",
		"character":    "Character",
		"collection":   "Collections",
		"contest":      "Contest",
		"custom_skill": "CustomSkills",
		"delivery":     "Delivery",
		"dungeon":      "Combat",
		"eventplaza":   "EventPlaza",
		"farm":         "Farm",
		"fleamarket":   "FleaMarket",
		"gemstore":     "GemStore",
		"god":          "God",
		"helper":       "Helpers",
		"home":         "Homes",
		"letter":       "Letters",
		"lottery":      "Lottery",
		"medal":        "Medals",
		"monster":      "Monster",
		"news":         "News",
		"notification": "Notifications",
		"park":         "Park",
		"party":        "Party",
		"player":       "Player",
		"pvp":          "Combat",
		"ranking":      "Rankings",
		"rescue":       "Rescues",
		"secretshop":   "Secret Shop",
		"shop":         "Shop",
		"system":       "System",
		"tavern":       "Tavern",
	}
	if tag, ok := tagMap[module]; ok {
		return tag
	}
	return strings.Title(module)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not find repository root containing go.mod")
}
