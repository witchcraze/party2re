package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
)

type Dependency struct {
	Name         string `json:"name"`
	TargetModule string `json:"target_module"`
	RelationType string `json:"relation_type"`
	SourceRef    string `json:"source_ref"`
}

type LockStep struct {
	Table string `json:"table"`
	Tier  int    `json:"tier"`
	Mode  string `json:"mode"`
	Note  string `json:"note"`
}

type TxBoundary struct {
	Method          string     `json:"method"`
	SourceRef       string     `json:"source_ref"`
	TransactionType string     `json:"transaction_type"`
	LockSequence    []LockStep `json:"lock_sequence"`
}

type ModuleDefinition struct {
	Schema                string       `json:"$schema"`
	Module                string       `json:"module"`
	Tier                  int          `json:"tier"`
	Description           string       `json:"description"`
	Package               string       `json:"package"`
	ImplementationPackage string       `json:"implementation_package"`
	Dependencies          []Dependency `json:"dependencies"`
	TransactionBoundaries []TxBoundary `json:"transaction_boundaries"`
}

type TemplateData struct {
	Module  ModuleDefinition
	AllMods []string
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Module.Module}} — Tier {{.Module.Tier}} Module Architecture | Party2Re</title>
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg: #020617;
      --card-bg: #0f172a;
      --border: #1e293b;
      --text: #f8fafc;
      --text-muted: #94a3b8;
      --text-dim: #64748b;
      --accent: #38bdf8;
      --accent-dim: #0284c7;
      --tier-bg: #0369a1;
      --code-bg: #090d16;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      font-family: 'JetBrains Mono', monospace;
      background: var(--bg);
      color: var(--text);
      line-height: 1.6;
      padding: 2rem;
    }
    .container {
      max-width: 1080px;
      margin: 0 auto;
    }
    header {
      border-bottom: 1px solid var(--border);
      padding-bottom: 1.5rem;
      margin-bottom: 2rem;
    }
    .nav-links {
      display: flex;
      gap: 1rem;
      font-size: 0.85rem;
      margin-bottom: 1rem;
    }
    .nav-links a {
      color: var(--accent);
      text-decoration: none;
      transition: color 0.2s;
    }
    .nav-links a:hover {
      text-decoration: underline;
    }
    .badge {
      display: inline-block;
      padding: 0.2rem 0.6rem;
      border-radius: 4px;
      font-size: 0.75rem;
      font-weight: 600;
      background: var(--tier-bg);
      color: #fff;
      margin-right: 0.5rem;
      vertical-align: middle;
    }
    h1 {
      font-size: 1.8rem;
      margin: 0.5rem 0;
      display: flex;
      align-items: center;
      gap: 0.75rem;
    }
    p.desc {
      color: var(--text-muted);
      font-size: 1rem;
      margin-top: 0.5rem;
    }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
      gap: 1.5rem;
      margin-bottom: 2rem;
    }
    .card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 1.25rem;
    }
    .card h2 {
      font-size: 1.1rem;
      color: var(--accent);
      margin-bottom: 1rem;
      border-bottom: 1px solid var(--border);
      padding-bottom: 0.5rem;
    }
    .meta-list {
      list-style: none;
      font-size: 0.85rem;
    }
    .meta-list li {
      margin-bottom: 0.5rem;
    }
    .meta-label {
      color: var(--text-dim);
      display: inline-block;
      width: 130px;
    }
    .code-ref {
      background: var(--code-bg);
      padding: 0.15rem 0.4rem;
      border-radius: 4px;
      border: 1px solid var(--border);
      color: #e2e8f0;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      margin-top: 0.75rem;
      font-size: 0.85rem;
    }
    th, td {
      border: 1px solid var(--border);
      padding: 0.6rem 0.75rem;
      text-align: left;
    }
    th {
      background: var(--code-bg);
      color: var(--text-muted);
      font-weight: 600;
    }
    .tier-badge {
      background: #1e293b;
      padding: 0.1rem 0.4rem;
      border-radius: 3px;
      font-size: 0.75rem;
    }
    .tx-card {
      background: var(--card-bg);
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 1.25rem;
      margin-bottom: 1.5rem;
    }
    .tx-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 0.75rem;
    }
    .tx-title {
      font-size: 1.1rem;
      font-weight: 600;
      color: #38bdf8;
    }
    .tx-type {
      font-size: 0.75rem;
      background: #334155;
      padding: 0.2rem 0.5rem;
      border-radius: 4px;
    }
    .mod-nav {
      display: flex;
      flex-wrap: wrap;
      gap: 0.5rem;
      margin-top: 2rem;
      padding-top: 1.5rem;
      border-top: 1px solid var(--border);
    }
    .mod-nav a {
      background: var(--card-bg);
      border: 1px solid var(--border);
      padding: 0.3rem 0.6rem;
      border-radius: 4px;
      color: var(--text-muted);
      text-decoration: none;
      font-size: 0.8rem;
    }
    .mod-nav a.active {
      border-color: var(--accent);
      color: var(--accent);
      font-weight: 600;
    }
    .mod-nav a:hover {
      border-color: var(--accent);
      color: #fff;
    }
  </style>
</head>
<body>
  <div class="container">
    <nav class="nav-links">
      <a href="../system-overview.html">← Back to System Architecture Overview</a>
    </nav>

    <header>
      <h1>
        <span class="badge">Tier {{.Module.Tier}}</span>
        <span>{{.Module.Module}} Module</span>
      </h1>
      <p class="desc">{{.Module.Description}}</p>
    </header>

    <div class="grid">
      <section class="card">
        <h2>Package Architecture</h2>
        <ul class="meta-list">
          <li><span class="meta-label">Domain Package:</span> <code class="code-ref">{{.Module.Package}}</code></li>
          <li><span class="meta-label">Implementation:</span> <code class="code-ref">{{.Module.ImplementationPackage}}</code></li>
          <li><span class="meta-label">Guidance Tier:</span> Tier {{.Module.Tier}} (Core Managed)</li>
        </ul>
      </section>

      <section class="card">
        <h2>Domain Dependencies</h2>
        {{if .Module.Dependencies}}
        <ul class="meta-list">
          {{range .Module.Dependencies}}
          <li>
            <span class="meta-label">{{.Name}}:</span>
            <code class="code-ref">{{.SourceRef}}</code>
          </li>
          {{end}}
        </ul>
        {{else}}
        <p style="color: var(--text-dim); font-size: 0.85rem;">No external domain dependencies.</p>
        {{end}}
      </section>
    </div>

    <section>
      <h2 style="font-size: 1.3rem; margin-bottom: 1rem; color: var(--text);">Deterministic Transaction Boundaries & Lock Hierarchy</h2>
      {{range .Module.TransactionBoundaries}}
      <article class="tx-card">
        <div class="tx-header">
          <div>
            <span class="tx-title">{{.Method}}</span>
            <span style="font-size: 0.8rem; color: var(--text-dim); margin-left: 0.5rem;"><code class="code-ref">{{.SourceRef}}</code></span>
          </div>
          <span class="tx-type">{{.TransactionType}}</span>
        </div>

        {{if .LockSequence}}
        <table>
          <thead>
            <tr>
              <th style="width: 50px;">Step</th>
              <th style="width: 160px;">Database Table</th>
              <th style="width: 80px;">Lock Tier</th>
              <th style="width: 140px;">Lock Mode</th>
              <th>Operation / Invariant Note</th>
            </tr>
          </thead>
          <tbody>
            {{range $index, $step := .LockSequence}}
            <tr>
              <td>{{add $index 1}}</td>
              <td><strong>{{$step.Table}}</strong></td>
              <td><span class="tier-badge">Tier {{$step.Tier}}</span></td>
              <td><code class="code-ref">{{$step.Mode}}</code></td>
              <td>{{$step.Note}}</td>
            </tr>
            {{end}}
          </tbody>
        </table>
        {{end}}
      </article>
      {{end}}
    </section>

    <footer class="mod-nav">
      <span style="color: var(--text-dim); font-size: 0.8rem; display: inline-flex; align-items: center; margin-right: 0.5rem;">Tier 1 Modules:</span>
      {{$current := .Module.Module}}
      {{range .AllMods}}
      <a href="{{.}}.html" class="{{if eq . $current}}active{{end}}">{{.}}</a>
      {{end}}
    </footer>
  </div>
</body>
</html>`

func main() {
	modulesDir := ".arch/modules"
	outDir := "docs/architecture/modules"

	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create out dir: %v\n", err)
		os.Exit(1)
	}

	files, err := filepath.Glob(filepath.Join(modulesDir, "*.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "glob modules failed: %v\n", err)
		os.Exit(1)
	}

	var allModNames []string
	var modules []ModuleDefinition

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read file %s failed: %v\n", file, err)
			os.Exit(1)
		}
		var mod ModuleDefinition
		if err := json.Unmarshal(data, &mod); err != nil {
			fmt.Fprintf(os.Stderr, "unmarshal %s failed: %v\n", file, err)
			os.Exit(1)
		}
		modules = append(modules, mod)
		allModNames = append(allModNames, mod.Module)
	}

	sort.Strings(allModNames)

	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}

	tmpl, err := template.New("module").Funcs(funcMap).Parse(htmlTemplate)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse template failed: %v\n", err)
		os.Exit(1)
	}

	for _, mod := range modules {
		outPath := filepath.Join(outDir, mod.Module+".html")
		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create %s failed: %v\n", outPath, err)
			os.Exit(1)
		}
		if err := tmpl.Execute(f, TemplateData{Module: mod, AllMods: allModNames}); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "render %s failed: %v\n", outPath, err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("Rendered %s\n", outPath)
	}
}
