# 架构

[English](ARCHITECTURE.md)

SDK 对外保持一套内聚的产品对象模型，对内按 Cloud 和 runtime 的所有权划分实现。应用
只需导入根 `agr` 包，也可以按需使用 `sandbox` 快捷包。

## 公共模型

```text
Client
  |-- SandboxManager -> Sandbox
  |                      |-- Files
  |                      |-- Commands
  |                      |-- PTY
  |                      |-- Code
  |                      `-- Metrics
  |-- ToolManager -> ToolInfo / ToolMutationResult
  |-- PreCacheImageManager -> PreCacheImageInfo
  |-- DeploymentManager -> DeploymentInfo
  |                       `-> DeploymentConnection -> HTTP / SSE / WebSocket
  |-- VolumeManager -> VolumeInfo
  |-- VolumeTemplateManager -> VolumeTemplateInfo
  |-- SessionSpaceManager -> SessionSpaceInfo
  |-- SessionManager -> SessionInfo / EventInfo
  `-- RegistryManager -> RegistryInfo
                         `-- RegistryRecordManager -> Record / Version / AuditLog
```

一个 `Client` 绑定一套腾讯云身份、地域、Endpoint 和请求策略。Create 和 Connect 都返回
唯一的 `*agr.Sandbox`。`sandbox` 包只是基于环境变量的 Client 快捷入口，不包含另一套
Sandbox 实现。

Preview SessionSpace 和 Session manager 是账号级纯控制面服务。Session 由 Space、User 和
Session 三部分共同标识，SDK 不补全默认值。状态与 Event 中的 JSON 由 SDK 校验大小和对象
结构。Session 不属于 Sandbox generation、CodeContext 或 Deployment affinity，也没有本地
`Close`；远端清理顺序是先删除 Session，再删除 SessionSpace。

Preview Registry manager 是账号级控制面服务。Record manager 必须绑定明确的 Registry ID；
Version 只能通过互斥的 Version ID 或 Label 选择。描述符、来源配置、Preview 结果和审计 JSON
是有界的 SDK 自有值。远程 `Preview` 是只读操作，`Sync` 是调用方显式发起的 mutation。
Skill 包传输使用独立的内部数据面：公共 API 只接收 `io.Reader`/`io.Writer`，短期预签名 URL
不进入公共模型。传输有大小上限、拒绝重定向、不转发 Cloud Authorization、下载时校验
SHA-256，并且从不解压归档。

Tool 和预热 manager 是账号级纯控制面服务。它们返回的描述、mutation 回执和任务状态是
SDK 自有的不可变值，不是已连接 handle：没有本地 generation，也不需要 `Close`。mutation
不会隐式触发 Describe，删除 Tool 也不会级联删除 Sandbox。

Deployment 控制操作同样返回不可变快照。`DeploymentConnection` 是唯一有状态的 Deployment
对象，固定绑定一个 Deployment、地域、domain 和 Tool 端口，负责 Token 延迟刷新、affinity
发现与轮换、活跃 HTTP body 和 WebSocket。`Close` 只取消本地操作，不触发 Cloud 删除。
公共 API 不暴露短期 Token，也不允许请求自行指定 authority。

`Client` 是 AGR 一级资源的稳定扩展点。新资源只有在具备公开控制面或数据面契约、明确的
所有权和生命周期、稳定错误、确定性测试、Cookbook 用户旅程及发布文档后才会加入。
推测性的资源名称和空 service interface 不进入公共 API。

## 依赖方向

```text
agr / sandbox（公共 facade）
        |
        +--> internal/controlplane --> 腾讯云官方 SDK
        |
        +--> internal/runtime --> internal/dataplane --> internal/gen
        |
        +--> internal/dataplane/deployment
        +--> internal/dataplane/registry
        `--> internal/model
```

- 根包负责公共选项、参数校验、稳定错误，以及公共模型和内部 DTO 的转换。
- `internal/controlplane` 负责 Cloud Client、签名集成、typed Action 映射、生命周期轮询、
  Metrics 请求、实例 Token 获取和新 runtime generation 的构造。
- Preview Volume、Session 和 Registry Action 仅通过内部白名单调用腾讯云官方 `common.CommonClient`。
  typed Action 失败后不会回退到 raw 路由，公共 API 也不暴露通用 RawCall。
- `internal/runtime` 负责 generation 有效性、请求生命周期、handle、stream pump、有界队列、
  输出聚合和本地失效。
- `internal/dataplane` 负责不可变 Endpoint 和实例访问材料、鉴权 Header、HTTP/Connect 调用、
  协议解析和 start barrier。
- `internal/dataplane/deployment` 负责 Deployment HTTPS/WSS 直连、并发 Token 刷新合并、
  affinity 顺序和连接级取消。WebSocket 依赖的类型只在内部使用。
- `internal/dataplane/registry` 负责有界的 Skill 包 HTTPS 传输，不在错误中保存签名 URL，
  不跟随重定向、不添加 Cloud Authorization，也不解压归档。
- `internal/gen` 保存生成的协议绑定，只允许数据面实现和协议测试夹具引用。
- `internal/model` 只保存多个内部层确实需要共享的最小 wire-independent DTO 和错误词汇，
  不 alias 公共类型或生成类型。

内部包不得反向导入根 `agr` 包。根目录生产代码不得导入 Connect、生成绑定或腾讯云生成 SDK。

## 生命周期所有权

每个已连接的 Sandbox 只拥有一个当前数据面 generation。Reader、Watch、Command handle、
PTY、Code execution 和受管理 Code context 都属于创建它们的 generation。

- Pause 在提交 Cloud mutation 前使当前 generation 失效。
- Resume 重新获取实例访问材料，并在数据面 ready 后安装新 generation。
- Close 同步终止本地工作，不删除远端实例。
- Delete 是显式远端清理操作。
- 调用方取消 context 只停止本地观察，不能证明已接受的远端副作用被撤销。

正常关闭流、generation 失效和显式远端进程信号是三种不同操作。跨层移动实现时不得合并
这些语义。

Deployment connection 使用独立且更简单的生命周期。`Connect` 先确认 Deployment 为 ACTIVE，
首次数据请求才获取 Token。SDK 在两小时安全窗口前刷新 Token，并合并并发刷新；刷新失败时
可以继续使用尚未过期的 Token。401 使 Token 缓存失效，但不会重放请求。SDK 从配置的响应
Header 学习 affinity ID，并串行处理首次发现和轮换，避免迟到响应恢复旧 ID。关闭 connection
会取消 HTTP/SSE/WebSocket 操作。

## 增加能力

1. 只有用户需要稳定契约时，才新增 SDK 自有公共输入和结果类型。
2. 私有 DTO 优先放在能力所属的内部包；只有多个内部层都需要时才放入 `internal/model`。
3. 为数据面增加语义操作，不暴露 Client、Endpoint、Token、生成消息或通用 Request。
4. 生命周期和有界异步行为放入 `internal/runtime`，公共 handle 只转换和委托。
5. 使用确定性的 loopback 测试覆盖映射、取消、上限、错误和远端副作用语义。真实 Cloud
   测试继续要求显式启用。
6. 运行 `make verify`。仓库检查器会验证依赖方向和根目录公共职责白名单。

公共 API 变化需要先讨论，并审查 API 快照、迁移说明和 Changelog。纯内部重构必须保持
公共 API 快照不变。

白名单和生成的控制面快照用于证明契约来源，不是第二套公共 API。两套 v2 manifest 记录
固定的 CLI revision、来源 channel、有效契约哈希，以及各 Action 所属资源、成熟度、route、
请求/响应类型和引用对象闭包。stable 读取基础 API。preview 委托 CLI 合成 patch，过滤未被
选择的 patch 字段，并且必须保持为 stable 的结构超集。Preview 通过语义化预发布版本交付，
不使用运行时开关或 Go build tag。
稳定和 Preview 公共 Go API 分别保存 snapshot。开发阶段由已评审 registry 选择当前源码对应
的 snapshot；发布阶段按 tag 再选择一次。

## 验证

默认验证完全离线，覆盖格式、单测、vet、race、公共消费者编译、API 快照、生成漂移、
仓库架构和敏感信息检查。可选真实 Cloud 验证及其环境变量见
[test/README.md](test/README.md)。

贡献流程见 [CONTRIBUTING-zh.md](CONTRIBUTING-zh.md)。
