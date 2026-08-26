---
name: Security Rules
description: Guidelines for ensuring security-sensitive changes are properly reviewed and handled.
---

# Security Principles

## 1. Security-Sensitive Changes Require Explicit Review
Changes that impact the security perimeter of the application must not be merged silently or superficially. Explicit review and attention are required for:
- Authentication / Authorization
- Secret handling and storage (passwords, tokens, API keys)
- Input validation and boundary enforcement
- SQL Injection, Command Injection, Path Traversal
- Cross-Site Scripting (XSS) and Cross-Site Request Forgery (CSRF)
- Server-Side Request Forgery (SSRF)
- Insecure deserialization
- Rate limiting and resource exhaustion protections
- Sensitive data logging (preventing PII or tokens from entering logs)

## 2. API Authorization Boundary
The presentation/HTTP layer is responsible for extracting session tokens and performing initial authentication. However, **authorization (ensuring the user has permission to access or modify a resource)** must be guaranteed at the Service/API boundary as well. Always ensure that the authenticated user (`Session.PlayerID`) matches the owner of the requested resource before the Service executes critical domain logic.

## 3. Dependency Vulnerabilities
Before introducing a new third-party dependency, verify its security posture and whether it introduces known vulnerabilities. Keep the dependency tree small to minimize attack surface.
