[Scope tangent] `cmd/landman/main.go` composes Union-specific jobs, commands, and schedules. Keep this one-county composition for this change. Move county composition out of the application entry point when a second county needs the same command surface.

[Unrelated code smell] Untracked `artifacts/verify/picomatch-upstream/node_modules` is inside the Go module tree. `go test ./...` discovers its Go package. Keep generated artifacts outside the module tree or exclude that path in a separate change.
