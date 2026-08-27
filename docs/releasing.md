# Releasing

Releases are built by GitHub Actions from signed or annotated version tags.

1. Ensure the default branch is green and the working tree is clean.
2. Update `CHANGELOG.md` under a new version heading.
3. Create and push a semantic version tag, for example `v1.1.0`.
4. The release workflow builds Linux, macOS, and Windows binaries, creates SHA-256 checksums, and publishes a GitHub release.
5. Download one artifact and verify its checksum and `version` output.

Do not rebuild or replace assets for an existing version. Publish a new patch version so consumers can audit immutable artifacts.
