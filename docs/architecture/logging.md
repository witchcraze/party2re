# Application Logging

Application logging is an infrastructure concern at the application boundary.
The initial implementation uses the standard-library `log/slog` package and
does not require an external dependency.

## Contract

Application services receive a small injectable logger contract. Log records
include an operation name, and callers may associate a correlation identifier
with a context using `logging.WithCorrelationID`. The logger emits structured
JSON suitable for container stdout/stderr.

Errors are recorded by concrete error type rather than error text. This keeps
diagnostic operation context while preventing database messages, DSNs, or
credentials from being copied into application logs.

## Secret safety

The logger drops attributes whose keys identify passwords, sessions, tokens,
credentials, authorization data, or database connection values. Nested groups
are filtered as well, and common `key=value` text is redacted. Application
code must not pass raw authentication values or domain objects containing
secrets to the logger.

Player account operations log only safe identifiers and outcome categories.
Authentication and session values are deliberately never logged. A service
logs an operation failure once; lower-level repositories do not duplicate the
same record.

## Boundaries

The logger has no global state and is not part of Core or a Feature Module.
Services depend only on its public contract. The command entry point creates
the JSON implementation for stderr; tests can inject a buffer-backed logger or
the no-op implementation.
