# Source attribution

## AGR Go SDK bootstrap

The initial implementation in this repository was migrated from the public
`TencentCloudAgentRuntime/ags-go-sdk` project at commit
`e21456e016ac9700bca6bd8bbdc51d3333f0447f`, under Apache-2.0. The migration
changed the module path and public package name, removed legacy-repository
release history, and repositioned `Client` as the AGR product entry point while
retaining Sandbox behavior and deterministic verification.

The source was imported through a tracked-file archive. Git metadata, local
validation output, credentials, resource identifiers, approval records, and
untracked development material were not copied. Subsequent modifications are
recorded in this repository's history and changelog.

## Filesystem and process protocols

The following files are derived from the E2B Infrastructure repository at revision
`97d10b529fd506f2ea3ac6a262a8b453fd8cbb14`, under Apache-2.0:

- `proto/tool/filesystem/filesystem.proto`
- `proto/tool/process/process.proto`

Tencent modified the copies in 2026 by adding an internal Go package mapping
and a prominent modification notice. The mapping was updated to
`github.com/TencentCloudAgentRuntime/agr-go-sdk` during the repository
migration. The current hashes and pinned generator versions are machine-readable in
[`contracts/proto.json`](contracts/proto.json).

The files under `internal/gen/` are generated from these modified protocol files by Buf,
`protoc-gen-go`, and `protoc-gen-connect-go`. Generated files must not be edited manually.

## Control-plane contract metadata

[`contracts/control-plane.json`](contracts/control-plane.json) is the reviewed SDK Action allowlist
and its static typed/raw routing decision. The versioned directory
under [`contracts/controlplane/ags/`](contracts/controlplane/ags/) contains a generated effective
snapshot limited to those Actions and the exact transitive closure of their request and response
objects. Its manifest records the pinned `ags-cli` repository revision, source `api.json` and
`api.patch.json` hashes, the compacted `apipatch render` hash, and the local snapshot hash.
Action and object documents are copied unchanged; only the non-wire `api_brief` is normalized so
it describes the allowlisted Sandbox scope rather than omitted API-key management Actions.

The full CLI schema and its patch are not copied into this repository. `ags-cli` remains the owner
of patch validation and rendering; the SDK synchronization command invokes that implementation
from an explicitly supplied checkout.

## Verification

`make verify-contracts` is offline. It checks the committed reduced snapshot hash, exact object
closure, field structure, manifest counts, Action routes, and production typed calls. It does not
claim to re-read the external source files.

Maintainers use `make check-control-contract CLI_DIR=../ags-cli` to compare the committed artifacts
with the pinned source checkout, or `make sync-control-contract CLI_DIR=../ags-cli` to regenerate
them after reviewing a source change. `make verify-generate` separately regenerates protocol code
with pinned tools in a temporary worktree and rejects unexplained drift.
