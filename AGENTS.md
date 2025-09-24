# Repository Guidelines

## Project Structure & Module Organization
- `kindle-weather.go` hosts the HTTP server, API clients, caching, and template rendering; keep glue code here and place substantial helpers in new files within the root module.
- `templates/` contains the Go HTML templates rendered for Kindle; `css/` and `font/` hold static assets served under `/css`.
- `k8s/` provides deploy-ready manifests (deployment, service, ingress, certificate); mirror this structure when adding environment-specific overlays.
- `kindle_test.go` exercises core formatting helpers; add new unit tests alongside the logic they cover.
- `.github/workflows/` stores CI automation; update workflows when introducing new build steps.

## Build, Test, and Development Commands
- `go run .` runs the server locally on port 8080 using configuration from `.secrets` or environment variables.
- `go build -o kindle-weather` creates the production binary; never commit the built artifact.
- `go test ./...` executes the Go test suite; use `go test -run TestName -v` to focus on a scenario.
- `docker build -t kindle-weather .` mirrors the deployment image; prefer this path before changing Kubernetes specs.

## Coding Style & Naming Conventions
- Format Go code with `gofmt` (tabs for indentation) before committing; `go fmt ./...` is the quickest catch-all.
- Follow idiomatic Go naming: exported identifiers in CamelCase with leading capitals, unexported helpers in mixedCaps.
- Keep template names descriptive (`*.tmpl`) and reference them via `templates/*.tmpl` globbing in code.
- Store runtime secrets in `.secrets` (OpenWeather, NOAA, SpaceDevs keys); never log their values.

## Testing Guidelines
- Use Go’s standard `testing` package; table-driven tests like those in `kindle_test.go` keep coverage focused.
- Include new tests for helpers that transform external API payloads or time formatting, since regressions there break the Kindle layout.
- Aim to keep `go test ./...` passing locally and in CI; add regression tests before touching caching or metrics code.

## Commit & Pull Request Guidelines
- Follow the existing Conventional Commit pattern (`feat:`, `ci:`, `fix:`, `docs:`) seen in recent history for clear changelog entries.
- Keep commits scoped and reference issue IDs in the body when applicable; include configuration changes with their code.
- PRs should describe the feature or fix, list validation steps (`go test ./...`, Docker build), and attach screenshots of visual changes rendered in the Kindle template if relevant.
