# Contributing

> Thanks for considering contributing to Twork

All contributions are welcome: bug fixes, new features, documentation improvements, refactoring, performance optimizations, or just reporting issues.

## Pull Requests

* Keep PRs focused on one change.
* Write clear commit messages.
* Update documentation if needed.
* Test your changes before submitting.

If you're planning a large feature, consider opening an issue first so we can discuss it.

## Building

`go build ./...` works on its own -- `internal/web` embeds a committed placeholder page so
the module always compiles. To build/run the real dashboard, build the frontend first:

```
npm --prefix web ci
npm --prefix web run build   # outputs into internal/web/dist
go build -tags sqlite_fts5 ./cmd/twork
```

Run `go test -tags sqlite_fts5 ./...` for the Go test suite; `npm --prefix web run lint`
covers the frontend.

## Coding Style

* Keep it simple.
* Prefer readable code over clever code.
* Avoid unnecessary dependencies.
* Stay consistent with the existing codebase.
* No external API, no AI additions

## Issues

Found a bug or have an idea?

Open an issue with as much detail as possible. Feature requests and discussions are always welcome.

## Be Respectful

Please keep discussions friendly and constructive.

Thanks for helping improve Twork.
