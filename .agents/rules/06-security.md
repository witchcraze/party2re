---
name: Security Rules
description: Rules for security-sensitive changes, authorization, credentials and dependencies.
---

# Security Rules

## 1. Security-Sensitive Changes Require Explicit Review
MUST NOT merge silently a change affecting: authentication/authorization, secret storage, input validation, injection (SQL, command, path traversal), XSS/CSRF, SSRF, deserialization, rate limiting/resource exhaustion, or logging of sensitive data.

## 2. API Authorization Boundary
HTTP authenticates; the Service/domain boundary MUST also verify that the session's `PlayerID` owns the resource before critical domain logic runs.

## 3. Mandatory Auth Helper and Common Wrapper Anti-Bypass Rule
- HTTP handlers taking `character_id` (path or body) or mutating character state MUST use `withAuthenticatedCharacter`, `withAuthenticatedCharacterAndJSON`, `authenticatePlayer` or `authenticateAdmin`.
- MUST NEVER decode `req.CharacterID` from a body without verifying `char.PlayerID == player.ID`.
- Enforced by `internal/architecture/auth_lint_test.go`.

## 4. Dependencies
Check a new dependency's known vulnerabilities before adding it (see `01-development-workflow.md` §6).

## 5. Credentials
- Passwords MUST use `bcrypt` with cost ≥ 12 (or `argon2id`). Custom SHA loops, PBKDF1, MD5, SHA-1, unsalted or fast digests are banned.
- Tokens, API keys and credentials MUST be compared in constant time (`subtle.ConstantTimeCompare`, `bcrypt.CompareHashAndPassword`).
- Tests of registration, hashing or authentication MUST use real production hashing. Other tests MAY reuse a pre-hashed template made once at production cost; MUST NOT lower the cost or add low-cost hooks to production APIs.
- Enforced by `internal/architecture/crypto_lint_test.go`.

## 6. Text Input Validation
- Every string Gateway parameter MUST declare a text kind in `internal/playercontext/catalog_text.go`; the Gateway rejects unsafe text by that kind before decoding. Enforced by `internal/playercontext/catalog_text_lint_test.go`.
- Legacy prohibited-character sets MUST come from the `internal/validation` profiles, chosen at the service boundary so non-HTTP callers get them too. Enforced by `internal/architecture/prohibited_chars_lint_test.go`.
