package http_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type RouteInfo struct {
	Method      string
	Path        string
	HandlerName string
}

// TestHTTPAuthenticationAndAuthorizationLinter performs AST static analysis on all
// HTTP API endpoints and handlers in internal/api/http/ to guarantee that:
//  1. All character-scoped routes (/characters/{id}/*) enforce authentication/authorization.
//  2. All mutating endpoints operating on character entities (e.g. eventplaza, shop, medals, etc.)
//     strictly route through withAuthenticatedCharacter, withAuthenticatedCharacterAndJSON, or authenticatePlayer.
//  3. All administrative routes enforce admin API key validation via authenticateAdmin.
//  4. Request payloads containing character identifiers cannot bypass ownership validation.
func TestHTTPAuthenticationAndAuthorizationLinter(t *testing.T) {
	httpDir := "."

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, httpDir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("failed to parse http package directory: %v", err)
	}

	httpPkg, ok := pkgs["http"]
	if !ok {
		t.Fatalf("package http not found in %s", httpDir)
	}

	// 1. Extract all route registrations from handler.go (inside Router() method)
	routes := extractRegisteredRoutes(t, httpPkg)
	if len(routes) == 0 {
		t.Fatalf("no routes extracted from Router()")
	}

	// 2. Index all handler functions across AST
	handlers := indexHandlerFunctions(httpPkg)

	// 3. Define public / unauthenticated route whitelist
	publicRoutePatterns := []*regexp.Regexp{
		regexp.MustCompile(`^GET /health$`),
		regexp.MustCompile(`^GET /openapi\.json$`),
		regexp.MustCompile(`^POST /players$`),
		regexp.MustCompile(`^POST /sessions$`),
		regexp.MustCompile(`^DELETE /sessions$`),
		regexp.MustCompile(`^GET /jobs$`),
		regexp.MustCompile(`^GET /medals/rewards$`),
		regexp.MustCompile(`^GET /helpers/quests$`),
		regexp.MustCompile(`^GET /news$`),
		regexp.MustCompile(`^GET /news/\{id\}$`),
		regexp.MustCompile(`^GET /park/posts$`),
		regexp.MustCompile(`^GET /park/npc/inspect$`),
		regexp.MustCompile(`^GET /eventplaza$`),
		regexp.MustCompile(`^GET /eventplaza/merchant/items$`),
		regexp.MustCompile(`^GET /eventplaza/banquets$`),
		regexp.MustCompile(`^GET /tavern/menu$`),
		regexp.MustCompile(`^GET /challenges/tiers$`),
		regexp.MustCompile(`^GET /auctions$`),
		regexp.MustCompile(`^GET /auctions/\{id\}$`),
		regexp.MustCompile(`^GET /fleamarket/listings$`),
		regexp.MustCompile(`^GET /fleamarket/listings/\{listing_id\}$`),
		regexp.MustCompile(`^GET /gemstore/catalog$`),
		regexp.MustCompile(`^GET /gemstore/recipes$`),
		regexp.MustCompile(`^GET /gemstore/dialogue$`),
		regexp.MustCompile(`^GET /god/dialogue$`),
		regexp.MustCompile(`^GET /rankings/.*$`),
		regexp.MustCompile(`^GET /homes/\{id\}$`),
		regexp.MustCompile(`^GET /homes/\{id\}/companion/talk$`),
	}

	adminRoutes := map[string]bool{
		"POST /news":             true,
		"POST /rankings/refresh": true,
	}

	for _, route := range routes {
		routeKey := route.Method + " " + route.Path

		// Admin routes must call authenticateAdmin
		if adminRoutes[routeKey] {
			fnDecl, exists := handlers[route.HandlerName]
			if !exists {
				t.Errorf("admin route %s maps to unknown handler %s", routeKey, route.HandlerName)
				continue
			}
			calls := extractFunctionCalls(fnDecl)
			if !containsAny(calls, "authenticateAdmin") {
				t.Errorf("admin route %s handler %s must invoke authenticateAdmin()", routeKey, route.HandlerName)
			}
			continue
		}

		// Public routes do not require authentication
		isPublic := false
		for _, re := range publicRoutePatterns {
			if re.MatchString(routeKey) {
				isPublic = true
				break
			}
		}
		if isPublic {
			continue
		}

		// All other routes MUST enforce authentication / authorization
		fnDecl, exists := handlers[route.HandlerName]
		if !exists {
			t.Errorf("authenticated route %s maps to unknown handler %s", routeKey, route.HandlerName)
			continue
		}

		calls := extractFunctionCalls(fnDecl)
		hasAuth := containsAny(calls,
			"withAuthenticatedCharacter",
			"withAuthenticatedCharacterAndJSON",
			"authenticatePlayer",
			"authorizeCharacter",
		)

		if !hasAuth {
			t.Errorf("security violation: route %s handler %s does not enforce authentication/authorization (calls: %v)",
				routeKey, route.HandlerName, calls)
		}
	}

	// 4. Verify request structs containing CharacterID field are handled via withAuthenticatedCharacterAndJSON
	verifyCharacterIDRequestPayloads(t, httpPkg, handlers)
}

func extractRegisteredRoutes(t *testing.T, pkg *ast.Package) []RouteInfo {
	t.Helper()
	var routes []RouteInfo

	for _, file := range pkg.Files {
		ast.Inspect(file, func(n ast.Node) bool {
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

				if len(call.Args) >= 2 {
					lit, ok := call.Args[0].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					pattern := strings.Trim(lit.Value, "\"")
					parts := strings.SplitN(pattern, " ", 2)
					if len(parts) != 2 {
						return true
					}

					var handlerName string
					switch h := call.Args[1].(type) {
					case *ast.SelectorExpr:
						handlerName = h.Sel.Name
					case *ast.Ident:
						handlerName = h.Name
					}

					if handlerName != "" {
						routes = append(routes, RouteInfo{
							Method:      parts[0],
							Path:        parts[1],
							HandlerName: handlerName,
						})
					}
				}
				return true
			})
			return false
		})
	}
	return routes
}

func indexHandlerFunctions(pkg *ast.Package) map[string]*ast.FuncDecl {
	handlers := make(map[string]*ast.FuncDecl)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				if strings.HasPrefix(fn.Name.Name, "handle") {
					handlers[fn.Name.Name] = fn
				}
			}
		}
	}
	return handlers
}

func extractFunctionCalls(fn *ast.FuncDecl) []string {
	var calls []string
	if fn == nil || fn.Body == nil {
		return calls
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			switch f := call.Fun.(type) {
			case *ast.SelectorExpr:
				calls = append(calls, f.Sel.Name)
			case *ast.Ident:
				calls = append(calls, f.Name)
			case *ast.IndexListExpr: // generic calls like withAuthenticatedCharacterAndJSON[Req]
				if id, ok := f.X.(*ast.Ident); ok {
					calls = append(calls, id.Name)
				}
			case *ast.IndexExpr: // single type argument generic call
				if id, ok := f.X.(*ast.Ident); ok {
					calls = append(calls, id.Name)
				}
			}
		}
		return true
	})
	return calls
}

func verifyCharacterIDRequestPayloads(t *testing.T, pkg *ast.Package, handlers map[string]*ast.FuncDecl) {
	t.Helper()

	// Find request structs with character_id JSON tag
	charReqTypes := make(map[string]bool)
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !strings.HasSuffix(ts.Name.Name, "Request") {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				for _, field := range st.Fields.List {
					if field.Tag != nil && strings.Contains(field.Tag.Value, "character_id") {
						charReqTypes[ts.Name.Name] = true
					}
				}
			}
		}
	}

	// Verify all handlers referencing these structs invoke withAuthenticatedCharacterAndJSON
	for handlerName, fnDecl := range handlers {
		calls := extractFunctionCalls(fnDecl)
		hasAuthHelper := containsAny(calls, "withAuthenticatedCharacterAndJSON", "withAuthenticatedCharacter", "authenticatePlayer")

		// Check if the handler uses any charReqType
		ast.Inspect(fnDecl.Body, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && charReqTypes[id.Name] {
				if !hasAuthHelper {
					t.Errorf("security violation: handler %s decodes %s (contains character_id) but does not use withAuthenticatedCharacterAndJSON",
						handlerName, id.Name)
				}
			}
			return true
		})
	}
}

func containsAny(slice []string, targets ...string) bool {
	for _, s := range slice {
		for _, t := range targets {
			if s == t {
				return true
			}
		}
	}
	return false
}

// Ensure unused import safety
var _ = filepath.Join
