# Release Process

1. Run `go test ./...`.
2. Run `make build`.
3. Run `make checksums`.
4. Validate `devsync version` includes expected metadata.
5. Run the validation matrix in `docs/validation-matrix.md`.
6. Create a snapshot with `make release-snapshot`.
7. Tag a semantic version when ready.

Build metadata uses:

```bash
go build -ldflags "-X github.com/danew/devsync/internal/buildinfo.Version=vX.Y.Z"
```
