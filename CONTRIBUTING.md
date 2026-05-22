# Contributing

Contributions are welcome. Please ensure tests pass and the project builds before submitting a PR.

## Development

```bash
# Install dependencies
go mod download

# Run tests
go test ./... -count=1

# Vet
go vet ./...

# Build
go build -o pdf2md .
```

## Submitting Changes

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Ensure `go test ./... -count=1` and `go vet ./...` pass
5. Open a PR with a clear description

## Reporting Issues

Please include your platform, PDF sample (if possible), and the model used when reporting conversion quality issues.