# Documentation map

During Version 1.0 reconstruction, the original Party2 project defines game behavior. The Go implementation describes what currently runs; a passing test or a document that matches Go is not evidence of behavioral parity. Approved modernization of transport, storage, and presentation must preserve the game rules unless a change is explicitly agreed.

| Location | Owns | How to verify |
| --- | --- | --- |
| [`AGENTS.md`](../AGENTS.md), [agent rules](../.agents/rules/) | Mandatory constraints and decision criteria | Check applicable rules before edits |
| [`STATUS.md`](../STATUS.md) | Major current capabilities, limitations and immediate gaps | Source, wiring, and current issue state |
| [`ROADMAP.md`](../ROADMAP.md) | Remaining milestones and planned direction | Open issues; distinguish plans from running features |
| [`architecture/`](architecture/) | Component boundaries and persistence contracts | Composition roots, public interfaces, migrations |
| [`design/README.md`](design/README.md) | Game rules, formulas, and state transitions | Original CGI behavior and explicitly approved changes |
| [`development/`](development/) | Repository workflows and validation | Makefile, scripts, hooks, CI |
| [`migration/legacy-cgi-mapping.md`](migration/legacy-cgi-mapping.md) | Navigation from legacy behavior to reconstruction targets | Legacy script, current source, and ticket evidence |
| [`migration/feature-inventory.md`](migration/feature-inventory.md) | Frozen reconstruction snapshot at its stated date/revision | Historical evidence; consult STATUS and open Issues for current gaps |
| GitHub Issues / PRs and Git history | Task progress and detailed change history | Current ticket state, merged PRs and commits |
| [`migration/documentation-audit.md`](migration/documentation-audit.md) | Known differences and unresolved reconstruction findings | Linked specifications, legacy evidence and current source |
| [`assets/`](assets/) | Original placeholder provenance and visual guidelines | Files and asset planning issues |
| [`api/base.json`](api/base.json), [`api/paths/`](api/paths/) | Current transport specification sources | Registered routes and `make openapi-check` |
| [`.arch/`](../.arch/) | Partial source navigation and transaction coordinates | Referenced symbols plus manual semantic review |

`api/openapi.json` and `internal/api/http/openapi.json` are generated bundles. Keep both because one is distributed as documentation and the other is embedded in the server; edit their modular sources.

Maintain documents when a change makes their explanations outdated. A routine implementation change does not require edits to every design/status document or an entry in the frozen inventory. Permanent documents explain contracts and decisions; detailed completion history is available in GitHub/Git. Evidence links for meaningful decisions remain with the owning explanation.

Current missing coverage and major limitations belong in STATUS or linked open Issues. Preserve known differences and unverified areas in their responsible design/audit documents; a frozen inventory is not current coverage certification.

When auditing documents, inventory files and references first, then compare statements with the appropriate authority above. Preserve correct legacy rules when Go differs and record the implementation gap with its source evidence. Label proposed contracts as planned, and obsolete designs as historical. Consolidate duplicate API tables and implementation inventories into their existing source of truth rather than maintaining multiple independent copies.

Local Markdown links and `.arch` symbol checks detect only structural drift. They do not validate gameplay formulas, every lock acquisition, or complete legacy action coverage. Verification and any unresolved specification gaps should be recorded in the maintenance issue or PR.
