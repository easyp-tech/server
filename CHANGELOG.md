# Changelog

## [Unreleased]

### Changed
- commit-id format change: the proxy now mints the first 16 bytes of the git SHA (with UUID version/variant bits) instead of the SHA-256 of the git SHA. Existing `buf.lock` entries are invalidated; clients must re-run `buf mod update` or `buf dep update` after upgrading.
