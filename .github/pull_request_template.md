## Summary

Describe the user-visible or maintainer-visible change and its motivation.

## Validation

- [ ] `make verify` passes with Go 1.22 and `GOTOOLCHAIN=local`.
- [ ] Public API, contract snapshots, `CHANGELOG.md`, and EN/ZH docs are updated where relevant.
- [ ] New behavior includes focused failure, cancellation, and lifecycle tests.
- [ ] No credentials, tokens, real resource identifiers, private responses, or real-cloud output are included.
- [ ] Real-cloud E2E was run when protocol or cloud behavior changed, or the reason it was not run is stated.

## Compatibility and ownership

- [ ] Remote deletion remains explicit; local `Close()` does not delete remote resources.
- [ ] Streams, queues, aggregates, and handles remain bounded and generation-bound.
- [ ] `LICENSE`, `NOTICE`, `THIRD_PARTY.md`, and `SOURCE_ATTRIBUTION.md` are updated if dependencies or copied sources changed.
