# Contributing

## Development

```bash
# Run the full CI check locally
make ci
```

This runs `go fmt`, `golangci-lint`, tests, and build.

## Submitting changes

1. Fork the repo and create a branch from `main`.
2. Make your changes and ensure `make ci` passes.
3. Use [conventional commits](https://www.conventionalcommits.org/) for your commit messages (e.g. `feat:`, `fix:`).
4. Open a pull request against `main`.
