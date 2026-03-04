# Releases

This project uses [GoReleaser](https://goreleaser.com/) for automated cross-platform builds and releases.

## Supported Platforms

- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64, arm64)

## Local Testing

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser/v2@latest

# Build snapshot (without publishing)
goreleaser build --snapshot --clean

# Full release dry-run
goreleaser release --snapshot --clean
```

## Creating a Release

1. Ensure all changes are committed
2. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
3. GoReleaser will automatically create a GitHub release with binaries for all platforms

Binaries will be available in the `dist/` directory after running GoReleaser locally, or as GitHub release assets when publishing.
