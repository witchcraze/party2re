## Summary

<!-- What does this PR change? Keep it concise. -->

## Related Issue

Closes #

## Affected Components

- **Packages / Modules**: `internal/<module>`
- **Database Tables / Migrations**: `migrations/<file>.sql` (or "None")

## Scope

### In scope

- 

### Out of scope

- 

## Tests

<!-- Describe tests added/changed and commands run. -->

- [ ] Focused tests pass
- [ ] Full test suite passes
- [ ] Active issue re-checked prior to commit and all acceptance criteria verified with zero omissions

## Legacy Specification Parity (Clean-room Audit)

<!-- Required for feature/refactor PRs reconciling or implementing legacy game mechanics -->
- [ ] **Legacy Reference**: Cited original Perl CGI path and line numbers (e.g. `party2/lib/guild.cgi:70-130`)
- [ ] **Action & Parameter Parity**: Verified `@actions`, formulas, state flags (`$m{...}`), and probabilities match the legacy reference
- [ ] **Zero Fictional Features**: Confirmed no divergent modern mobile-game/RPG conventions (unauthorized level-ups, fees, or instant shortcuts) were introduced

## Architecture

- [ ] No architectural impact
- [ ] Architecture/design documentation updated
- [ ] Architecture review completed, if required

## Dependencies / licenses

- [ ] No new dependency or asset
- [ ] New dependencies/assets were reviewed for license and provenance

## Documentation / status

<!-- Assess which explanations this change makes outdated under
     .agents/rules/02-documentation-sync.md. List affected owner documents,
     or briefly explain why no update is needed. The frozen inventory is
     historical reference; do not append per-Issue/PR completion records.
     Briefly explain meaningful consolidation or necessary growth and its owner. -->

- [ ] Documentation impact assessed
- [ ] Affected explanations/contracts updated in this PR, or no update needed (explain below)

Documentation content check ([rule 02](https://github.com/witchcraze/party2re/blob/main/.agents/rules/02-documentation-sync.md)):

- [ ] Documents keep only: approved decisions and their rationale; unresolved specifications and known gaps linked to open Issues; verified rules not yet implemented; non-obvious findings (source defects, unreachable or broken branches, misleading names or values) with legacy line references
- [ ] Documents contain none of: content already expressed by code, tests or OpenAPI (formulas, parameters, shapes, error codes, counts); hand-maintained lists that have a registry or schema; status, progress or completion history, including links to closed Issues/PRs that only say where something was implemented; a second copy of a rule (link its owner); legacy line references that only locate a routine findable by name

## Notes

<!-- Anything reviewers should know. -->
