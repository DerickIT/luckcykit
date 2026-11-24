# 微信云账户、点数与 API Token 生产闭环

- 编号：`005`
- 状态：实施中
- 负责人：待确认
- 创建时间：2026-11-24
- 关联 Issue / PR：无

## 1. 背景与问题

当前仓库有独立 cloud loopback 沙箱，包含邮箱 OTP、沙箱点数订单、追加式账本和托管模型用量结算，但没有微信 OAuth、真实微信支付、生产数据库或用户 API Token 管理。旧 LuckyTV 外部连接不是本项目生产云账号实现，且生产源已停用。

## 2. 目标与非目标

### 目标

- 使用微信开放平台网站应用扫码登录建立独立云账号。
- 使用 PostgreSQL 作为生产权威数据源，SQLite 仅保留沙箱和测试。
- 支持安全的 Web Cookie 会话、桌面一次性 handoff code 和用户 API Token。
- 使用微信 Native 扫码支付购买永久点数，异步通知验签后入账。
- 由独立云 Web 提供用户中心和运营后台。
- 保持点数预留、用量结算、追加式账本和未知状态不自动重试的不变量。

### 非目标

- 不迁移现有沙箱账号、点数或旧外部连接余额。
- 不恢复 `https://luckytv.app` 旧外部源。
- 本阶段不写入或使用真实微信 AppSecret、商户私钥或线上账号。
- 没有生产公网、Secret、商户和数据库条件时不宣称生产联调完成。

## 3. 用户场景与需求

- 作为云端创作者，我希望用微信扫码登录独立云账号。
- 作为云端创作者，我希望购买永久点数并按托管模型实际 token 用量消费。
- 作为 API 使用者，我希望创建、命名、查看元数据和撤销有限期 API Token，明文只显示一次。
- 作为运营管理员，我希望维护版本化商品、查询订单/账本并登记外部退款冲正，全部操作可审计。

## 4. 范围与边界

- 涉及服务/数据：`backend/cmd/cloud`、`backend/internal/cloud/**`、独立 `cloud-web/`、云 API 文档和生产配置。
- 外部系统或依赖：微信开放平台 OAuth、微信支付 Native、PostgreSQL、HTTPS 反向代理、Secret 环境。
- 权威数据源：云 PostgreSQL；账号、会话、API Token、商品、订单、支付事件、账本、预留和任务均由服务端决定。
- 身份：生产必须使用 UnionID；微信 provider identity 独立建号，不按昵称、头像或邮箱自动合并。
- Token：`Authorization: Bearer lkt_...`，仅允许读取自己的账户、提交托管任务和读取自己的任务；每账号最多 10 个，默认 90 天，可选 7/30/90/365 天。
- 支付：Native 扫码；只有验签成功的异步通知确认到账；商品金额使用人民币分和正整数点数，订单冻结商品快照。
- 退款：首期整笔全额退款，仅限未消费点数；先锁定，再由运营在微信商户平台完成退款，系统登记退款单号并追加冲正，禁止负余额。
- 明确不改变：本地 workspace、owner token、BYOK、旧本地数据和旧 LuckyTV 外部连接路径。

## 5. 验收标准

- [x] 生产配置在缺少 PostgreSQL、微信 AppID/Secret、HTTPS callback 或管理员初始化 Secret 时拒绝启动；真实生产凭据未配置。
- [x] 微信 OAuth state 短期且一次性；fake provider 强制 UnionID；回调地址要求 HTTPS。
- [x] Web 会话使用 Secure、HttpOnly、SameSite=Lax Cookie；桌面 handoff code 短期、绑定客户端 state、单次兑换，Session Token 不进入 URL。
- [x] API Token 明文只在创建响应出现一次；数据库只保存哈希、前缀、末四位、有效期和审计元数据；撤销/过期后立即拒绝。
- [ ] Native 支付通知签名、商户、金额、订单和幂等性校验通过后才追加正向账本。
- [ ] 退款锁定、外部退款登记、冲正账本和审计在事务中幂等；余额不足拒绝，不产生负余额。
- [x] PostgreSQL 生产迁移与 SQLite 沙箱迁移互不混淆；沙箱数据不迁移。
- [ ] 云 Web 用户/管理员会话和 API 权限边界独立；管理员高风险操作需 MFA/重新验证并审计。

## 6. 约束与决策

- 已确认决策：微信开放平台网站应用；UnionID 必须存在；PostgreSQL 生产；独立 cloud Web；Native 支付；永久点数；外部人工退款登记；管理员四角色和生产 TOTP MFA；环境 Secret 注入。
- 待确认问题：生产域名、微信开放平台应用配置、商户号/证书/平台证书、PostgreSQL 连接与备份、Secret Manager、SMTP/通知和正式运营负责人。
- 兼容性/迁移约束：已发布沙箱迁移不可改写；生产首次使用独立数据库；不迁移既有沙箱数据。
- 安全、隐私、费用、频率或合规约束：密钥不入仓库、日志、URL 或普通响应；API Token 每账号最多 10 个，默认每 Token 每分钟 60 次、每账号并发任务 3 个；未知上游状态保留预留。
- 依赖或禁止事项：禁止无真实验签的支付到账；禁止客户端计算余额、价格或扣费；禁止把桌面 owner/launch token 作为云认证。

## 7. 技术方案

### 职责与数据流

`cloud-web/ -> cloud HTTP -> identity/payment/ledger/task/admin -> PostgreSQL`。微信 OAuth 只由服务端发起和兑换；支付回调只由服务端验签并进入订单/账本事务。桌面通过一次性 handoff code 与云服务兑换会话，不把会话秘密放进 URL。

### API / 数据 / 配置变更

- API：新增微信授权、回调、handoff、Cookie 会话和 API Token 合同；保留沙箱端点仅供开发。
- 数据库迁移：追加 OAuth state、handoff、API Token、支付通知、退款锁定/冲正和限流结构；生产 PostgreSQL 使用独立迁移路径。
- 配置与部署：生产必须配置 PostgreSQL、微信 OAuth callback/AppID/Secret、HTTPS 代理边界、支付密钥和管理员 MFA/Secret 环境。
- 文档：同步 HTTP API、数据库、部署和用户/运营页面文档。

### 失败、权限和恢复

OAuth state、handoff、验证码和会话均短期、一次性或可撤销；支付/退款按外部事件 ID 和业务订单幂等；账本只追加；支付或上游状态未知时不自动重试、不按零成本处理；管理员操作保留审计和恢复依据。

## 8. 实施任务

- [x] 实现 PostgreSQL 生产存储和 fail-closed 配置
- [x] 实现微信 OAuth、UnionID 身份和 Web/桌面会话 handoff
- [x] 实现用户 API Token、scope、过期、撤销；跨实例限流后续实现
- [x] 实现订单商品金额快照、统一支付确认事务和沙箱适配；微信 Native 验签/通知、商品后台和退款后续实现
- [ ] 实现独立 cloud Web 用户中心和运营后台
- [ ] 实现桌面接入与 fake/生产配置验收
- [ ] 更新 API、数据库、部署文档并执行安全检查

## 9. 验证计划与结果

| 验证项 | 结果 | 证据/说明 |
| --- | --- | --- |
- 自动化测试 | 已通过 | `go test ./internal/cloud/... ./cmd/cloud/...`：51 项通过；覆盖 OAuth state、UnionID、Cookie、handoff code 和现有云沙箱行为 |
- 构建/静态检查 | 已通过 | `node scripts/structure-budget.mjs`；`node scripts/deadcode-budget.mjs`：当前 647，基线 647 |
- 手工/端到端验收 | 未执行 | 未配置公网微信、支付、PostgreSQL 或生产 Secret |
- 安全与数据检查 | 部分通过 | 生产配置 fail-closed、Secret 不写响应/日志的代码路径及云/本地迁移隔离已测试；真实部署和证书轮换未验证 |

## 10. 风险与开放问题

- 风险：微信审核产品、UnionID 授权范围、支付证书轮换和生产数据库备份尚未由真实部署验证。
- 缓解措施：fake provider/payment、fail-closed 配置、不可变账本、幂等约束和独立生产数据目录。
- 开放问题及决策人：生产域名、运维部署、微信和商户资质、数据库备份责任人、MFA/Secret Manager 方案。

## 11. 变更记录

| 日期 | 变更 | 原因/决策 |
| --- | --- | --- |
| 2026-11-24 | 建立并批准生产闭环规格 | 明确微信登录、API Token、支付、账务、生产数据库和云 Web 边界 |
