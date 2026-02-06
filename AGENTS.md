# AGENTS.md (Agentic Coding Rules)

This repository is used with AI coding agents/tools. Any agent operating in this repo must follow these rules unless explicitly overridden by the user.

## Feature vs Chore
- **Feature**: any change that introduces or modifies user-visible behavior, business logic, API behavior, request/response shapes, persistence, concurrency behavior, authorization, error semantics, or anything that could reasonably break existing behavior.
  - **Features must meet the full Definition of Done** below (DI + unit tests + passing tests).
- **Chore**: changes that are intended to be non-functional (docs, comments, formatting-only, renames with no behavior change, build/CI tweaks, minor refactors that preserve behavior).
  - Chores should still keep the repo healthy, but **do not require new unit tests** unless the chore touches logic in a way that could affect behavior.

If it’s unclear whether a request is a Feature or a Chore, **ask the user to clarify before making changes**.

## Definition of Done (features)
A feature request is only considered complete when **all** of the following are true:

1) **Go-style dependency injection is used** so the change is unit-testable:
   - External interactions (time, randomness, filesystem, network, DB, queues, OS env, HTTP clients, logging sinks) must be behind **interfaces** or passed-in function types.
   - Avoid new global variables for dependencies. Avoid hidden singletons.
   - Prefer **constructor injection** (e.g., `NewX(deps...)`) and store dependencies on structs.
   - Keep interfaces small and defined in the **consumer** package (where they’re used) unless there’s a clear shared contract.
   - Keep diffs minimal: only refactor what’s necessary to make the code testable.

2) **Unit tests exist and are easy to write using injected deps**:
   - Add/extend tests for new behavior using **only the Go standard library** (`testing`, `net/http/httptest`, etc.).
   - Prefer simple **handwritten fakes/stubs** that implement injected interfaces.
   - Tests must not require real network/DB/filesystem unless explicitly requested.
   - Avoid sleeps; inject time/clock to make tests deterministic.

3) **Tests pass locally**:
   - Run `make test` and fix failures before considering the task complete.

If any of the above cannot be satisfied, STOP and explain what blocks completion and propose a DI-friendly design that would satisfy the requirements.

## DI conventions (Go)
Use these patterns unless the user requests otherwise:

- **Constructor injection**
  - `type Service struct { dep Dep }`
  - `func NewService(dep Dep) *Service { return &Service{dep: dep} }`
  - Prefer constructor-based injection for config and dependencies.

- **Function injection** (good for tiny seams)
  - `type Now func() time.Time`
  - `type RandIntn func(n int) int`

- **HTTP seam** (when mocking `http.Client`)
  - `type HTTPDoer interface { Do(*http.Request) (*http.Response, error) }`

- **Time seam**
  - Inject `func() time.Time` or a `Clock` interface for deterministic tests.

- **Testing DI practices**
  - Use interfaces for external dependencies and provide small handwritten fakes/stubs in the test file.
  - Replace dependencies directly on structs in tests when needed (e.g., swap HTTP clients).
  - Avoid DI frameworks; keep DI manual and explicit.
  - Reset shared registries/state in tests to keep isolation.

## Testing guidelines (stdlib only)
- Prefer table-driven tests where it improves clarity.
- Name tests clearly: `Test<Type>_<Method>_<Scenario>`.
- Use `t.Helper()`, subtests (`t.Run`), and `errors.Is` / `errors.As` for error checks.
- Verify meaningful behavior/state; avoid brittle string-matching of entire error messages.
- Keep fakes close to tests (in the same `*_test.go` file) unless reused widely.

## Work process expectations
For feature work:
1) Briefly state the plan and identify DI seams / interfaces.
2) Implement production code with injected dependencies.
3) Add unit tests using fakes.
4) Run `go test ./...` and report results.
5) Summarize what changed (files/functions) and any follow-ups.

## Safety & scope
- Do not modify vendored code, generated code, or dependencies unless explicitly requested.
- Do not run destructive commands.
- Do not introduce third-party libraries without explicit user approval.
- Keep changes focused on the request.
