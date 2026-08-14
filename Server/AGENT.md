# Go Server Agent Contribution Guide

## Scope and Precedence

- These rules apply to Go server code in this directory and its descendants.
- Follow the repository-root rules first, then this file. A deeper `AGENTS.md` may add narrower rules.
- When conventions are unclear, inspect code, tests, the Makefile, and development documentation. Do not substitute memory or guesses for verification.
- Use the relevant skill or project documentation for database, DDD, gRPC, observability, CI/CD, and deployment details. Keep only required quality and boundary rules here.

## Before Editing

- Read two or three nearby files and tests in the same package. Preserve existing directories, filenames, types, and error-handling patterns.
- Use `make help`, README, CONTRIBUTING, or development documentation to identify the supported format, test, lint, build, and generate commands.
- Confirm that the change belongs to an existing requirement and task. When it does not, follow the root rules and add the required planning documents first.
- Fix the root cause. Do not hide problems by disabling lint, weakening tests, ignoring errors, or introducing broad type assertions.

## Code Style

- Use the project's formatter and static-analysis commands, such as `make fmt` and `make lint`.
- Comments explain why, not what. Record only constraints, historical context, policy references, compatibility, or exceptions. Use complete sentences.
- Keep assertion messages short and make test names provide the primary context.
- When supported by the Go version, prefer standard-library helpers such as `slices`, `maps`, and `cmp` over duplicate helpers or unnecessary dependencies.
- New files use the package already used by their directory. Package names are lowercase, single words, and contain no underscores; avoid `util` and `common`.
- Group imports as standard library, third-party packages, and project packages. Do not keep unused imports or introduce import cycles.
- Use tabs, UTF-8 without BOM, and one trailing newline.
- Add godoc comments for exported functions, types, and constants according to project convention.
- Do not add comments that merely restate code behavior.

## Code Quality and Error Handling

- Keep functions focused, use early returns, and avoid unnecessary nested `else` blocks.
- `context.Context` is the first parameter at I/O, HTTP, database, queue, and external-service boundaries, and it must be propagated.
- Do not store `context.Context` in structs or introduce arbitrary `context.Background()` calls on request paths.
- Handle errors immediately. Add context with `fmt.Errorf("context: %w", err)` when useful, and use `errors.Is` or `errors.As` across layers.
- Return expected failures as `error`. Do not use `panic`, `log.Fatal`, or `os.Exit` for normal flows in libraries, handlers, or workers.
- Error strings start with lowercase letters and have no trailing punctuation unless the project convention differs.
- Follow Go initialism conventions such as `URL`, `HTTP`, `ID`, and `API`.
- Avoid `any`, broad type assertions, mutable global state, and unmanaged goroutines. If unavoidable, document the reason in the smallest scope.
- Prefer clear public APIs and useful zero values. Use functional options only when complexity and existing conventions justify them.

## Concurrency, Resources, and HTTP Clients

- Every goroutine needs an exit mechanism such as `context.Context`, `WaitGroup`, or channel closure, and must react to `ctx.Done()` during shutdown.
- Protect shared mutable state with `sync.Mutex`, `sync.RWMutex`, or a measured lock-free design.
- Place `defer Close()` in the appropriate scope. Do not accumulate defers in large loops; use a nested function when necessary.
- Copy slices or maps when they cross mutable ownership boundaries and could otherwise be shared unexpectedly.
- HTTP clients retain only configuration and reusable `*http.Client` instances, never per-request state.
- Use `http.NewRequestWithContext`, set a reasonable timeout, and close `resp.Body` after reading it.
- Reuse transports. Retry only idempotent operations with backoff, limits, and explicit retryable-error rules.
- Before rereading `req.Body`, create a safe copy and set `GetBody`. Keep `io.Pipe` and multipart writing synchronized with clear close ownership.

## JSON, Time, and Data Boundaries

- External input and output types use consistent `json` tags. Viper-backed configuration types also use `mapstructure` tags.
- Use `omitempty` only when the API contract permits it; do not change response semantics merely to omit fields.
- Reject unknown external JSON fields when compatibility allows it, so spelling mistakes are not ignored silently.
- Store and calculate time in UTC. Use RFC3339 for JSON and format for the user's time zone only at presentation boundaries.
- Apply size and format limits to file input. Protect archive handling from Zip Slip and uncontrolled expansion.

## API, Input, and Security

- Validate all external input for format, length, range, enumeration values, and authorization.
- Preserve existing request, response, error-code, and authorization semantics when changing endpoints. Obtain explicit approval for breaking changes.
- Use existing DTOs, validation, error handling, and authorization middleware. Do not create inconsistent handler-level contracts.
- Update OpenAPI, frontend schemas/types, specifications, and tests together with API changes.
- Do not log passwords, tokens, connection strings, personal data, or other sensitive information. Error responses must not expose SQL, stacks, or internal configuration.
- Use parameterized queries. Never concatenate user input into SQL, shell commands, file paths, or URLs.
- Use only standard `crypto/*` for cryptography. Assess regexes, file handling, and external URLs for ReDoS, path traversal, and SSRF risks.

## Configuration, DI, and API Documentation

- Configuration management must use `spf13/viper`. Define sources and precedence explicitly; environment variables override config files, and config files override defaults.
- Unmarshal into a typed Config and validate centrally. Do not read Viper throughout business code.
- Optional settings require explicit defaults. Required settings must be validated at startup.
- Test environment-variable mappings, nested keys, and empty-value behavior to avoid environment-specific surprises.
- Inject secrets through environment variables, Secrets, or managed configuration services. Do not store them in repositories, examples, or logs.
- Missing or invalid required configuration must fail startup explicitly and return to the unified lifecycle entry point. Do not continue with unsafe implicit defaults.
- Compose infrastructure and application dependencies in an explicit composition root. When using `uber-go/fx`, centralize it in `cmd/` or a clearly named DI root.
- Keep repository and service interfaces small and capability-focused. Keep concrete implementations unexported and create test doubles using project conventions.
- Treat OpenAPI as a traceable HTTP contract. Update the OpenAPI source and regenerate output whenever endpoints, DTOs, validation, or error responses change.
- In services using `swag`, update Swagger comments and run the project-defined `swag init` command. Do not hand-edit generated files.

## When Using gRPC

- Keep proto files in the project-defined location. Never hand-edit generated code.
- Regenerate and commit code after RPC changes. Servers must honor client deadlines and `context.Context` cancellation.
- Preserve interceptor order: recovery, observability, logging, authentication/authorization.
- Implement gRPC health checking and map domain errors to correct gRPC status codes.
- Shut down with `GracefulStop` or a timeout-bound stop flow so active RPCs are not interrupted unnecessarily.

## When Using Domain Events

- Events are immutable structs containing at least an event ID, occurrence time, aggregate ID, and event type.
- Use past-tense names such as `OrderCreated` and `PaymentCompleted`.
- Synchronous events may remain within one bounded context; asynchronous cross-context events require explicit message boundaries.
- With the Outbox Pattern, write business changes and events in the same database transaction. Publishing workers must retry and track completion.
- Consumers must handle duplicates idempotently using the event ID or an equivalent de-duplication key.

## Lifecycle and Logging

- Manage HTTP server, worker, consumer, and background-goroutine lifecycles with `github.com/vincent119/commons/graceful`.
- Work started after startup must be cancellable by `context.Context`. Stop accepting new work first, wait for active work, then close database, cache, queue, trace, and other external resources.
- Do not call `os.Exit` or `log.Fatal` in server goroutines or libraries. Return errors to the unified lifecycle entry point.
- All application logging must use `github.com/vincent119/zlogger`. Do not mix standard `log`, `slog`, `zap`, `logrus`, or other loggers.
- Use structured logs and preserve existing correlation fields such as `trace_id`, `span_id`, `req_id`, and `subsystem`.
- Use existing OpenTelemetry configuration and propagate `context.Context` across every boundary.
- Use consistent snake-case metric names and unit suffixes such as `_total`, `_seconds`, and `_bytes`. Do not use high-cardinality labels such as `user_id`, email, or `trace_id`.
- Do not write secrets, personal data, DSNs, or complete external responses to logs or metric labels.

## Tests, Generated Artifacts, and Documentation

- For new observable behavior or a bug fix, prefer a failing test first, then the smallest viable fix, then refactoring.
- Every bug fix requires a regression test that reproduces the issue before the fix and passes after it.
- Test names describe observable behavior and outcomes, not private implementation details.
- Prefer table-driven tests. Use `*_test.go` and follow the project's test-package convention.
- Mark helpers with `t.Helper()`, clean up with `t.Cleanup()`, and run `-race` when concurrency or races need verification.
- Prefer isolated tests for domains, services, parsers, validators, transformations, and query planners. Repository, HTTP, database, and external-service integration tests validate boundary contracts.
- Add benchmarks and fuzz tests for high-risk algorithms or parsers when appropriate; they are not mandatory for every small change.
- Do not weaken assertions, delete or skip tests, or move core logic outside tests merely to make tests pass.
- Documentation, comments, formatting-only changes, and mechanical changes fully covered by existing tests do not require test-first work, but still require proportionate existing checks.
- Regenerate and commit files generated from schemas, APIs, or code using project commands.
- If verification cannot run, state the reason, unverified scope, and risk explicitly.
- Run `git diff --check` before handoff.

## Data and Migrations

- Do not run schema migrations during application startup.
- Production must not use ORM automatic schema migration, such as GORM `AutoMigrate`.
- Use versioned migrations for schema changes. Never modify a migration applied in any environment.
- Before adding indexes, views, materialized views, aggregate tables, or backfills, measure query plans, data volume, lock risk, and expected benefit.
- Migrations, backfills, schedules, and rollbacks require traceable deployment procedures. Project tooling and documentation define the details.
- When existing schema, data, migrations, or configuration differs from these rules, report the gap, impact, and migration plan. Do not make broad incidental repairs unless the task includes them or the user approves them.

## Documentation, Changes, and Handoff

- Update appropriate documentation for user-facing feature, API, configuration, deployment, or operational changes.
- Integrate content into the existing documentation structure such as `docs/`, README, OpenAPI, runbooks, or the private specification workspace. Do not create duplicate parallel documentation.
- Documentation describes verified current behavior; never state planned or unverified behavior as fact.
- Keep each change small and focused. Do not bundle unrelated refactors.
- Explain and obtain confirmation before adding dependencies, changing architecture, changing shared configuration, or changing deployment behavior.
- At handoff, report changed files, behavioral impact, validation results, and migration, configuration, or deployment notes.
- Commit and PR text accurately describes verified scope and never labels plans, tools, or unverified outcomes as complete.

## Project Tooling and Migration Contract

- Define the Go version from `go.mod`.
- Define the OpenAPI generation command and output path in the repository.
- If Swagger, Fx, gRPC, Proto generation, or an automatic migration tool is not adopted, state that explicitly and define the required adoption contract before introducing it.
- Use `github.com/vincent119/commons/graceful` for graceful shutdown and `github.com/vincent119/zlogger` for application logging.
- Define migration location, numbering convention, deployment order, rollback responsibility, and observability tooling in the project documentation.
- Apply unapplied versioned SQL migrations before application deployment; the API startup flow must not run them.
