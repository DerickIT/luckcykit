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

2026-11-21 扫描 `go list -m all` 仍有以下模块命中公告。逐项确认：`go list -deps ./cmd/desktop ./cmd/server ./cmd/luckytv ./cmd/cloud` 对这些模块的导入数为 0，`go mod why -m` 报告主模块不需要它们。它们来自上游模块的 `go.mod`（如 Wails 桌面框架、goreleaser/gonum 等工具链依赖），提升版本会被 `go mod tidy` 剪回。

| 模块 | 当前版本 | 公告数 | 说明 |
| --- | --- | --- | --- |
| `github.com/nats-io/nats-server/v2` | v2.5.0 | 42 | 仅模块图；不参与构建 |
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
| `github.com/blakesmith/ar` | 2019 版 | 1 | 来自 Wails 打包工具；仅模块图 |
| `rsc.io/pdf` | v0.1.1 | 1 | 来自 gonum/plot 工具链；仅模块图 |

处置方式：该列表用于 GitHub Dependabot 告警的逐条解释与风险评估。若要彻底消除，需要升级或移除上游工具链依赖（Wails、goreleaser、gonum），属于独立改动；在此之前不改 `go.mod` 钉版本，避免与 `go mod tidy` 打架。

## 5. 变更记录

| 日期 | 变更 |
| --- | --- |
| 2026-11-21 | 建立基线：修复可达漏洞（otel/sdk），加入 govulncheck 与 `bun audit` 门禁，登记模块图残留公告 |
