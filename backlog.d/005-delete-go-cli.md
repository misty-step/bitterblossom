# Delete Go bb CLI

Priority: low
Status: ready
Estimate: S

## Goal
Remove `cmd/bb/` entirely. One language (Elixir), one surface. The Go transport was transitional; all its functionality now lives in conductor Elixir code or is unnecessary.

## Oracle
- [ ] `cmd/bb/` directory deleted
- [ ] No references to `cmd/bb/` in conductor code
- [ ] `go build` commands removed from docs
- [ ] CI updated if it builds Go

## Notes
Corresponds to open issue #703.
