# 从 `ags-go-sdk` 迁移

[English](MIGRATING_FROM_AGS_GO_SDK.md)

本文以已发布的 `ags-go-sdk` **v0.1.5** 为旧版基线。该版本与旧仓库 `main`
同为提交 `351c2e5`。旧仓库未发布的 `codex/sdk-v0-convergence` 分支已经改变了
公共 API，不属于本文的对比范围。生产项目迁移时，请固定已发布的 `agr-go-sdk`
版本。

`agr-go-sdk` 使用新的 Go module 路径和根包 `agr`，不提供旧版深层包的兼容入口。
两个 module 可以同时出现在迁移中的依赖图里，但不能混用两边的 Sandbox、连接、
凭证、生成类型和命令句柄。

## 必须修改的调用

| `ags-go-sdk` v0.1.5 | `agr-go-sdk` |
| --- | --- |
| `github.com/TencentCloudAgentRuntime/ags-go-sdk/...` | `github.com/TencentCloudAgentRuntime/agr-go-sdk`；常用快捷入口在 `.../sandbox`。旧 `sandbox/core`、`sandbox/code`、`tool/*`、`connection`、`constant`、`pb/*` 路径没有同名替代包。 |
| `sandbox/code.Create(ctx, toolName, options...)` | `sandbox.Create(ctx, sandbox.CreateOptions{Tool: sandbox.ToolRef{Name: toolName}})`；已有 Tool ID 用 `ToolRef{ID: id}`。 |
| 每次调用传入 `WithClient(*ags.Client)`、`WithCredential`、`WithRegion` | 用 `agr.NewClient(...)` 创建显式 Client，再调用 `client.Sandboxes()`；单身份也可用读取环境变量的 `sandbox` 快捷入口。 |
| `sb.SandboxId`、`sb.Files`、`sb.Commands`、`sb.Code` | `sb.ID()`、`sb.Files()`、`sb.Commands()`、`sb.Code()`。 |
| `sb.Kill(ctx)`、包级 `Kill(ctx, id)` | `sb.Delete(ctx)`、`client.Sandboxes().Delete(ctx, id)`。`sb.Close()` 只释放本地资源，不删除云端实例。 |
| `sb.GetHost(port) string` | `sb.GetHost(port) (string, error)`，端口不合法时返回错误。 |
| `List(ctx)` 返回生成的 `[]*ags.SandboxInstance` | `List(ctx, agr.SandboxListOptions{})` 返回一页 `SandboxPage`；跨页使用 `ListPage` 或 `Iterate`。 |
| `SetTimeoutSeconds(ctx, seconds)` | `Update(ctx, agr.UpdateOptions{Timeout: &duration})`；Update 的 SDK 下限为 300 秒。 |

旧版 `WithDataPlaneDomain("example.com")` 接收基础域名，并在前面拼地域。
新版 Sandbox 使用 `agr.WithSandboxDomain("ap-guangzhou.example.com")`，传入值是
**完整的数据面域名后缀**；它同时影响 `GetHost` 和 SDK 内部的 Sandbox 连接。
新版 `WithDataPlaneDomain` 只配置 Deployment 数据面，控制面地址另由
`WithControlPlaneEndpoint` 配置。自定义域名还需要服务端 DNS、TLS 和路由支持。

旧版的 `WithScheme("http")` 和可修改的 `connection.Config` 没有对应的公共入口。
旧版可用实例 Token 直接构造 `tool/filesystem`、`tool/command`、`tool/code`
Client；新版只提供 Cloud 凭证管理的 Sandbox 连接，由 SDK 获取实例访问材料。
如果项目依赖独立 Token、手工指定数据面 URL 或额外的数据面 Header，这部分
不能直接迁移。`WithHTTPClient` 可提供受信任的 HTTP 传输设置，但不恢复上述入口。

## 行为变化

- 旧版 `Connect` 获取实例 Token 后返回；新版先读取状态，可能恢复暂停的实例或
  延长运行时长，并等待数据面就绪。只读取实例信息时改用 `Sandboxes().Get`。
- 旧版创建沙箱默认请求 5 分钟。新版 `CreateOptions.Timeout == nil` 由服务端选择
  默认值；要保持原行为，应显式传 `5 * time.Minute`。旧版字符串形式的
  `SandboxConfig.Timeout` 改为 `*time.Duration`，挂载参数改为 `[]agr.MountOption`。
- 旧版命令在没有 `ProcessConfig.Args` 时自动执行 `/bin/bash -l -c <命令>`。
  新版直接传递可执行文件和参数。保留旧行为可调用
  `sb.Commands().Run(ctx, "/bin/bash", agr.CommandOptions{Args: []string{"-l", "-c", "echo hello"}})`。
- 旧版命令的 `OnOutputConfig` 回调改用 `Start` 后读取 `CommandHandle.Events()`。
  `Pid` 字段改为 `PID()`；`SendInput(ctx, pid, data)` 改为 `Write(ctx, data)`；
  `Disconnect` 改为仅结束本地观察的 `Close()`；远程结束进程用
  `Signal(ctx, agr.SignalKILL)`。命令的 stdout、stderr 和 Code 结果有大小上限。
- 旧版文件 `Read` 返回自动在 EOF 关闭的 `io.Reader`；新版返回 `io.ReadCloser`，
  完整读取和提前退出都要调用 `Close()`。`Rename` 改为 `Move`，`GetInfo` 改为
  `Stat`；`MakeDir`、`Move`、`Write` 的返回值也改成 SDK 自有的 `FileInfo`。
- 旧版 `RunCodeConfig.ContextId` 字符串改为 `RunCodeOptions.Context`。
  `CreateContext` 返回的上下文绑定实例和代际，Pause/Resume 后不能复用。
  如需让服务端检查已有 ID，显式使用 `agr.NewExternalCodeContextRef(id)`；
  它不保证该上下文存在或仍有效。`RunCode` 改为 `Code().Run`，输出从
  `Execution.Logs.Stdout/Stderr` 移至 `CodeExecution.Stdout/Stderr`。
- 错误可用 `errors.As(err, &sdkError)` 提取 `*agr.Error`，依据 `Code` 和
  `Reason` 分类。创建或修改请求结果不明时，先查询资源，不要盲目重试。

示例见 [默认创建与清理](examples/cookbook/01_default/main.go)、
[显式 Client 与域名](examples/cookbook/02_explicit_client/main.go) 和
[Code 上下文](examples/cookbook/03_code_context/main.go)。英文指南包含完整的
调用对照和代码片段。

## 迁移验收

1. 先找出所有旧 module 引用、直接 Token 连接、手工域名与 Scheme、生成模型和
   protobuf 用法，为没有直接替代入口的调用确定新接入方式。
2. 固定已发布的 AGR 版本，替换导入路径和公共类型，使用消费者自己的 Go
   版本完成编译与离线测试。
3. 检查命令的 Shell 行为、输出上限、文件 Reader 和句柄关闭、`Connect`
   的生命周期影响，以及实例的显式 `Delete`。
4. 涉及真实云生命周期或数据面的项目，再做独立授权的云端验证。确认清理成功，
   日志中没有凭证或实例访问材料。

`ags-cli` 仍使用旧 module 的深层包；它的迁移需要单独的下游改动。
