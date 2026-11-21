# 依赖安全基线

- 状态：持续维护；记录依赖漏洞的处理范围、门禁与残留风险。
- 关联：[质量门禁](../../.github/workflows/quality.yml) · [Dependabot 配置](../../.github/dependabot.yml) · [需求 002](../requirements/002-current-product-baseline.md)

## 1. 判定口径

依赖漏洞分两类，处理方式不同：

- **可达漏洞**：`govulncheck` 能从 `backend/cmd/*` 的调用图证明存在调用路径。这类必须修复，CI 门禁失败。
- **模块图漏洞**：只出现在 `go list -m all` / `bun.lock` 的依赖图里，任何交付二进制或页面都不导入。Go 的模块裁剪会让这类条目由上游模块的 `go.mod` 决定，`go mod tidy` 会把手工提升的版本剪回上游要求的最低版本，因此不能靠长期钉版本消除。

前端（npm）与 Go 都以“可达/已解析集合”为准，不看单一模块图的条目数量。

## 2. 当前门禁

`.github/workflows/quality.yml` 的 `dependency-security` 作业在每次 PR/push 运行：

- `go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...`：Go 可达漏洞扫描。
- `bun audit --audit-level=high`（`web`、`agent-host`，使用 `registry.npmjs.org`，因为镜像源不提供 advisories 接口）：npm 已解析依赖的安全审计。

本地等价命令：

```bash
cd web && BUN_CONFIG_REGISTRY=https://registry.npmjs.org bun audit
cd agent-host && BUN_CONFIG_REGISTRY=https://registry.npmjs.org bun audit
cd backend && go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...
```

## 3. 已修复

| 依赖 | 变更 | 原因 |
| --- | --- | --- |
| `go.opentelemetry.io/otel/sdk` | `1.46.0` → `1.47.0` | GHSA-q7gw-24fr-5p69（CVE-2026-128085）：属性值由合法 U+FFFD 组成时可绕过 `AttributeValueLengthLimit`；`internal/supporttrace` 实际导入该模块 |
| `golang.org/x/oauth2` | `0.35.0` → `0.36.0` | 随 otel/grpc 依赖图更新 |
| `google.golang.org/protobuf` | `1.36.11` → `1.36.12` | 同上 |

## 4. 残留告知（模块图内、未被交付物导入）

2026-11-21 扫描 `go list -m all` 仍有以下模块命中公告。逐项确认：`go list -deps ./cmd/desktop ./cmd/server ./cmd/luckytv ./cmd/cloud` 对这些模块的导入数为 0，`go mod why -m` 报告主模块不需要它们。提升版本会被 `go mod tidy` 剪回，因此不钉版本。

| 模块 | 当前版本 | 公告数 | 说明 |
| --- | --- | --- | --- |
| `github.com/nats-io/nats-server/v2` | v2.5.0 | 43 | 仅模块图；不参与构建 |
| `google.golang.org/grpc` | v1.40.0 | 12 | 仅模块图；不参与构建 |
| `github.com/golang-jwt/jwt/v4` | v4.0.0 | 4 | 仅模块图；不参与构建 |
| `github.com/aws/aws-sdk-go`（v1） | v1.40.45 | 2 | 仅模块图；构建使用 v2 |
| `github.com/golang/glog` | 2016 版 | 2 | 仅模块图 |
| `github.com/hashicorp/go-retryablehttp` | v0.5.3 | 2 | 仅模块图 |
| `github.com/nats-io/jwt` | v1.2.2 | 2 | 仅模块图 |
| `github.com/prometheus/client_golang` | v1.11.0 | 2 | 仅模块图 |
| `github.com/sirupsen/logrus` | v1.8.1 | 2 | 仅模块图 |
| `github.com/klauspost/compress` | v1.18.7 | 1 | 仅模块图 |
| `go.etcd.io/etcd/client/pkg/v3` | v3.5.0 | 1 | 仅模块图 |
| `github.com/blakesmith/ar` | 2019 版 | 1 | 仅模块图 |
| `rsc.io/pdf` | v0.1.1 | 1 | 仅模块图 |

### 4.1 根因链

`go mod graph` 显示这些公告只有两个源头：

| 源头 | 依赖链 | 覆盖的公告模块 |
| --- | --- | --- |
| `github.com/volcengine/volc-sdk-golang`（Volcengine Ark 私有素材鉴权，`base` 包） | `volc-sdk-golang@v1.0.257` → `go-kit/kit@v0.12.0` → nats-server、nats-jwt、prometheus/client_golang、logrus、klauspost/compress、aws-sdk-go v1、golang-jwt/jwt v4、etcd client pkg；`go-kit` → `prometheus/common`、`armon/go-metrics` → go-retryablehttp；`go-kit` → `hdrhistogram-go` → `gonum/plot` → `rsc.io/pdf` | 除下一行的全部 |
| `github.com/wailsapp/wails/v3`（桌面宿主框架） | `wails/v3@v3.1.0` → `goreleaser/chglog` → `blakesmith/ar` | `blakesmith/ar` |

`google.golang.org/grpc` 的版本由 MVS 决定权在这些模块手里（`cloud.google.com/go/*`、`go-kit` 等），单独提升会被 `go mod tidy` 剪回 v1.40.0。

### 4.2 已评估的消除方案

- **升级 Volcengine SDK**：`v1.0.259`（最新）仍要求 `go-kit/kit v0.12.0`，升级无法断链。
- **移除 Volcengine SDK**：仓库只在 `internal/generation/ark_private_assets.go` 与 `internal/generation/protocol_auth.go` 使用 `volc-sdk-golang/base` 做签名与鉴权。改为本地实现意味着重写厂商签名逻辑，且缺少真实凭据无法验证，风险高于收益，本轮不采用。
- **升级 Wails**：`v3.1.0` 是当前版本，`goreleaser/chglog` 仍在其依赖内。
- **钉死版本**：与 `go mod tidy` 冲突，不可持续。

结论：在更换上述两个上游依赖前，这 13 个模块的公告属于可解释的残留。GitHub 上可按“受影响代码不作为依赖引入（vulnerable code not in path）”逐条 dismiss。

### 4.3 复现命令

```bash
node scripts/dependency-risk-report.mjs                  # 列出命中项与是否参与构建
node scripts/dependency-risk-report.mjs --fail-on-imported  # 构建内有命中则退出非零
```

## 5. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-11-21 | 建立基线：修复可达漏洞（otel/sdk），加入 govulncheck 与 `bun audit` 门禁，登记模块图残留公告 |
