# Goose Go version

**Out-of-scope decision.** Pinned `GOOSE_VERSION` to `v3.30.0` instead of leaving `latest`. That tag is what `latest` resolved to on 10 Nov 2026, and `go install` of it on `golang:1.26-alpine` printed `goose version: v3.30.0`. Leaving `latest` lets the next goose release require a newer Go and break the same targets. Did not pin `v3.27.3` to stay on Go 1.25, and did not adopt the `v3.26.0` shell pin from the pull request 14 comment. Justin chose the 1.26 image. Did not run an arena. The install runs already separated those options.

**Skipped refactor.** Left the existing comments in the Makefile and `.env.example`. They are outside the image and version change.

**Out-of-scope decision.** Updated `.env.example` to the same image and version. `-include .env` runs before `?=`, so a copied example overrides the Makefile. Did not change that precedence. An existing `.env` that still sets `golang:1.25-alpine` or `latest` keeps the old behavior.

**Skipped refactor.** Did not remove the `go install` redirect that drops stderr. `make migrate-up` on the old image exits with recipe error 1 and does not print the Go version line. Showing that line would make the next break obvious. It is a separate change.

**Unrelated.** The migrate recipes echo `DATABASE_URL`, including the password, before goose runs.

**Out-of-scope decision.** Did not edit `.cursor/skills/verify-landman/`. Pull request 22 is on main. Those files do not name `golang:1.25-alpine` or a `GOOSE_IMAGE` workaround.

**Skipped.** Did not add a failing test commit. A check that installs goose in Docker is integration-heavy. The failing command output is the repro. `go test -race ./... -count=1` does not cover this Makefile path.

**Skipped.** `control-cli` and `deslop` are not installed. The live check used `docker compose` and `make`.
