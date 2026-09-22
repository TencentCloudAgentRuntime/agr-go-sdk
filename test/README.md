# Real Cloud E2E

This nested module is an explicit, serial test boundary. Root `go test ./...` does not enter it
and never accesses Cloud services.

Set all variables before running:

```bash
export AGS_E2E=1
export TENCENTCLOUD_SECRET_ID=...
export TENCENTCLOUD_SECRET_KEY=...
export AGS_E2E_REGION=ap-guangzhou
export AGS_E2E_TOOL_ID=...       # custom envd-capable Tool used for Files, Commands, PTY, and Metrics
export AGS_E2E_CODE_TOOL_ID=...  # code-interpreter Tool used for Code and context lifecycle
export AGS_E2E_PRECACHE_IMAGE=... # authorized enterprise TCR or personal CCR image
export AGS_E2E_PRECACHE_REGISTRY_TYPE=enterprise # or personal
make test-e2e
```

`TENCENTCLOUD_TOKEN` is optional for temporary Cloud credentials. The tests never accept an API
key or a Sandbox instance token. `AGS_E2E_CONTROL_ENDPOINT` optionally overrides the official
Cloud endpoint for an authorized test environment; omit it for the public endpoint.

The suite is serial (`-p=1`). Each stable journey creates at most five Sandboxes, one uniquely
named Tool, and one uniquely named Deployment. The Preview persistent-Volume journey creates one
additional bootstrap Tool/Sandbox to initialize and later remove its dedicated COS prefix or CFS subpath. Every
journey registers independent cleanup for each accepted creation. Cleanup sends Delete and polls
until the control plane reports
`STOPPED` or `NOT_FOUND`. A failed cleanup fails the test. Do not run it against a shared Tool
unless you are authorized to create and delete test instances.

The A0+A1 journey creates a Tool with a unique `agr-sdk-e2e-*` name, waits for ACTIVE, updates it,
creates and removes one Sandbox from it, and then deletes only that Tool and polls until NotFound.
It also starts and observes one pre-cache task. The test account therefore needs Tool CRUD,
Sandbox CRUD/data-plane, and image pre-cache permissions. The image must already be readable by
the account; the SDK does not create or delete a registry image. The test rejects unverified
`custom` registry input and accepts `enterprise` or `personal` only.

The Deployment journey is intentionally verified in Shanghai and requires
`AGS_E2E_REGION=ap-shanghai`. It uses `AGS_E2E_TOOL_ID` as a shared envd-capable Tool; it never deletes the
Tool. It creates a scale-to-zero Deployment, exercises Create/Get/List/Update, connects directly
to port 49983, sends a ConnectRPC JSON `Filesystem/ListDir` request, closes the local connection,
then proves that the Deployment remains ACTIVE. Independent cleanup submits asynchronous deletion
and polls until exact NotFound, so a cleanup failure fails the test.

To run only the Deployment journey:

```bash
cd test
go test -v -count=1 -run '^TestDeploymentControlAndHTTPCloudJourney$' .
```

Its minimal environment is `AGS_E2E=1`, the two Tencent Cloud secret variables,
`AGS_E2E_REGION` set to `ap-shanghai`, and `AGS_E2E_TOOL_ID`.

To run only this A0+A1 journey, `AGS_E2E_TOOL_ID` and `AGS_E2E_CODE_TOOL_ID` are not required:

```bash
cd test
go test -v -count=1 -run '^TestToolCursorAndPreCacheCloudJourney$' .
```

The minimal environment for that command is `AGS_E2E=1`, the two Tencent Cloud secret variables,
`AGS_E2E_REGION`, `AGS_E2E_PRECACHE_IMAGE`, and `AGS_E2E_PRECACHE_REGISTRY_TYPE`.

The Preview persistent-Volume journey always runs in `ap-guangzhou`; it does not use
`AGS_E2E_REGION`. Configure exactly one reviewed storage fixture:

```bash
# COS option
export AGR_VOLUME_COS_ENDPOINT=<bucket-appid>.cos.ap-guangzhou.myqcloud.com
export AGR_VOLUME_COS_BUCKET=...         # reviewed Bucket; never deleted by the test

# CFS option instead of COS
export AGR_VOLUME_CFS_FILESYSTEM_ID=...  # reviewed ap-guangzhou CFS; never deleted by the test

# Preferred for custom Tools: copied read-only; never modified or deleted by the test
export AGR_VOLUME_BASE_TOOL_ID=...

# Built-in alternative instead of AGR_VOLUME_BASE_TOOL_ID
export AGR_VOLUME_TOOL_TYPE=code-interpreter

export AGR_VOLUME_TOOL_ROLE_ARN=...      # optional override for the copied or built-in Tool
export AGR_VOLUME_STORAGE_ROLE_ARN=...   # optional; defaults to the copied Tool role
cd test
go test -v -count=1 -run '^TestPersistentVolumeCloudJourney$' .
```

The test records the initial Volume and VolumeTemplate ID sets, creates only uniquely named
resources under `agr-sdk-e2e/<run-id>`, and cleans up in dependency order. A bootstrap mount
creates that unique COS prefix or CFS subpath before Volume creation and removes only its contents
at the end. The test deletes Sandboxes, its Tools, derived and standalone Volumes, and then its
VolumeTemplates; it never deletes the Bucket or CFS file system and never reuses an unreviewed
account resource. A base Tool is queried by exact ID only; its runtime configuration is copied to
the two test-owned Tools, while the base Tool itself remains unchanged. The fixture and base Tool
must belong to `ap-guangzhou`; cross-region resources are invalid.
The COS endpoint must identify the reviewed Bucket, not the region-wide
`cos.ap-guangzhou.myqcloud.com` endpoint. `SandboxManager.Delete` stops an instance and leaves a
`STOPPED` record; the journey therefore verifies both reclaim-policy values but does not claim
that Stop triggers the service's delete-on-Sandbox-deletion behavior.
Missing or ambiguous fixture variables fail before any network request instead of selecting an
existing resource from List.

The Preview Session journey always runs in `ap-shanghai` and needs no Tool or storage fixture.
Its minimal environment is `AGS_E2E=1` and the two Tencent Cloud secret variables:

```bash
cd test
AGS_E2E=1 go test -v -count=1 -run '^TestSessionCloudJourney$' .
```

It creates two uniquely named non-default SessionSpaces and the same Session ID under different
Space/User identities. It verifies CustomState, StateDelta, structured Event content,
Author/AfterTimestamp filtering, pagination, updates, isolation, and consistent EventCount. It
deletes only those Sessions, then their SessionSpaces, and confirms each resource is NotFound.

The explicit-client journey covers Files, streamed upload/read, Watch, Commands Run/List/Connect,
PTY, all ten Metrics with an explicit Start/End window, Update, and Connect. A dedicated
code-interpreter journey covers Code, managed/external contexts, Pause/Resume, and generation
invalidation. Keeping the Tool IDs separate avoids claiming that an arbitrary custom image
provides the Code service on port 49999. A separate default-shortcut journey proves the
environment-backed entrance. The 30-second creation boundary has its own resource and cleanup
path.
