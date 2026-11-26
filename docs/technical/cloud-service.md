# 好运TV云服务技术设计

- 状态：持续更新的技术设计；当前产品范围见 [需求 004](../requirements/004-cloud-hosted-model-oneapi.md)，历史讨论见 [需求 001](../requirements/001-wechat-auth-hosted-model-service.md)。
- 当前云服务仍是 loopback SQLite 沙箱；邮箱 OTP/API、OneAPI 固定 Chat Completions 数据面、任务预留/usage 结算及 admin bootstrap/session/RBAC 基础已实现并由 fake/行为测试验证。SMTP、OneAPI、模型、支付未真实联调；管理员运营 API 与生产部署未完成。
- 上位文档：[架构方案](cloud-service-architecture.md)。

## 1. 技术边界

本设计只规定服务间和领域合同，不锁定尚未确认的数据库、云厂商、微信授权产品或支付宝产品。建议采用独立云服务入口和云数据存储；不要复用桌面 `workspace` 数据库作为云账号库。沿用 Go 生态与现有 Provider/Protocol 概念是否合适，需以云部署和依赖验证后决定。

建议调用层次：

```text
HTTP/router -> handler/DTO -> application use case
            -> identity | catalog/pricing | payment | ledger | modelgateway/usage
            -> repository / provider adapters
```

`handler` 做协议解析与响应，不含账务策略；领域模块不直接互相写表；数据库事务由应用层协调，Ledger 拥有额度分录写入权。

## 2. API 草案

统一使用带版本前缀的云 API，例如 `/api/v1`。成功/失败 envelope、错误码与分页风格应与 LuckyTV API 约定协调，但云认证、权限和公网安全配置独立。以下是资源草案，方法、字段和 OAuth 回调方式还需 API review。

### Email OTP Identity（沙箱实现）

`POST /api/v1/auth/email/code` 接收邮箱并返回中性 accepted 响应；`POST /api/v1/auth/email/verify` 校验一次性验证码并创建 cloud session。验证码只持久化 HMAC 摘要，使用 fake mailer 或显式 SMTP over TLS 发送；生产组合根不挂载邮箱 OTP 路由，邮箱入口仅用于本地沙箱和测试。

### WeChat Identity（当前生产登录实现）

桌面先在本机随机 loopback 端口创建短期回调地址，再请求 `GET /api/v1/auth/wechat/start?handoffURL=...&clientState=...`。服务端校验回调地址只能是 loopback HTTP、保存一次性 OAuth state，并返回微信授权 URL。微信回调 `GET /api/v1/auth/wechat/callback` 验证 state、兑换 code、强制 UnionID；桌面回调只接收短期 handoff code，不接收云 session。

| 操作 | 端点 | 说明 |
| --- | --- | --- |
| 建立微信授权 | `GET /api/v1/auth/wechat/start` | 服务端生成短期 state；桌面请求必须同时提供 loopback `handoffURL` 与 `clientState` |
| 微信回调 | `GET /api/v1/auth/wechat/callback` | 兑换 code、校验 UnionID；桌面场景重定向到一次性 handoff URL |
| 交换桌面会话 | `POST /api/v1/auth/wechat/handoff` | 接收 `{code,state}`，校验绑定 state，单次兑换为 cloud session |
| Web 会话 | 微信回调直接设置 `Secure`、`HttpOnly`、`SameSite=Lax` Cookie | 无 handoff 参数时使用；session token 不进入 URL |
| 刷新/退出 | `POST /api/v1/sessions/revoke` | 当前实现支持会话撤销；刷新策略仍待后续设计 |

生产必须配置 `LUCKYTV_CLOUD_ALLOWED_ORIGINS`，使用逗号分隔的精确 origin（例如 `https://cloud.example.com`）；服务端只允许这些 origin 的 CORS 请求并支持 credentials，拒绝 `*` 和未列出的来源。开发沙箱可按需配置本地 WebView origin。

### Catalog、Orders、Ledger

| 操作 | 草案端点 | 说明 |
| --- | --- | --- |
| 查询模型目录 | `GET /api/v1/catalog/models` | 返回登录可用模型、能力和展示信息；不含上游密钥 |
| 查询额度商品 | `GET /api/v1/catalog/credit-products` | 服务端权威金额、币种、额度、有效期 |
| 创建支付订单 | `POST /api/v1/payment/orders` | 客户端传 product ID 和支付方式，不传可信金额 |
| 查询订单 | `GET /api/v1/payment/orders/{id}` | 当前账号归属校验 |
| 支付通知 | `POST /api/v1/payment/notifications/{provider}` | 验签、幂等、金额/币种核对；返回平台要求的确认格式 |
| 退款/冲正 | 内部管理端点待定 | 强权限、幂等并生成反向账本记录 |
| 查询额度汇总/明细 | `GET /api/v1/credits`、`/entries` | 服务端账本投影，明细分页 |

### Hosted model tasks

| 操作 | 草案端点 | 说明 |
| --- | --- | --- |
| 提交托管生成 | `POST /api/v1/model-tasks` | 会话授权、模型权限、幂等键和额度预留 |
| 查询任务 | `GET /api/v1/model-tasks/{id}` | 账号归属、状态与结果引用校验 |
| 取消任务 | `POST /api/v1/model-tasks/{id}/cancel` | 仅在供应商支持时取消上游；返回结算状态 |
| 查询用量 | `GET /api/v1/usage` | 只返回用户有权查看的用量/费用摘要 |

桌面本地任务 API 和云端任务 API 不共用本地 owner token。具体任务流式传输方式（轮询/SSE）待网络环境和部署选择后确定。

## 3. 核心数据模型草案

字段类型、索引、加密策略与数据库方言在技术选型后再定。金额用明确币种的最小货币单位或定点十进制，不用二进制浮点。

- `accounts`: 云账号 ID、状态、创建/关闭时间。
- `account_identities`: `provider`、provider subject、account ID、绑定/撤销时间；`provider + subject` 唯一，原始授权 code 不持久化。
- `auth_sessions`: session ID、state hash、poll credential hash、状态、过期时间、一次性兑换时间；秘密只保存哈希或短期必要材料。
- `cloud_sessions`: 会话/刷新凭据哈希、account ID、权限、过期/撤销时间、设备信息最小化记录。
- `catalog_versions` / `model_prices`: 模型权限、协议、定价版本、生效范围和变更历史。
- `credit_products`: 产品版本、金额、币种、授予额度、有效期及销售状态；下单后订单保存不可变快照。
- `payment_orders`: 订单 ID、account ID、product snapshot、应付金额/币种、状态、过期时间、幂等键。
- `payment_events`: provider event ID、订单 ID、验证结果、通知摘要、到达/处理时间；provider event ID 唯一。
- `ledger_entries`: account ID、分录类型、数量、币种/单位、关联订单或任务、幂等键、冲正引用、创建时间；追加写，不覆盖历史。
- `model_tasks`: account ID、client idempotency key、模型/价格版本、状态、额度预留、供应商 request reference、结果引用与时间戳。
- `usage_records`: task ID、供应商 usage、计量单位、成本快照、结算状态；不存不必要的提示词/素材原文。
- `audit_events`: 操作者、动作、对象、结果、脱敏上下文和追踪 ID。

跨账号对象读取必须由服务端按 `account_id` 约束；不可把随机 ID 当唯一授权措施。

## 4. 订单与账本状态

```text
Order: created -> pending_payment -> paid -> credited
                    |                  |
                    -> expired          -> refund_pending -> refunded/partially_refunded

Credit reservation: none -> reserved -> settled
                               |           -> released
                               -> released
```

- 所有转换需校验前置状态，支付事件和订单状态在一个数据库事务内协调。
- 重复支付通知返回既有处理结果，不产生第二份入账。
- 退款先记录退款事件；额度回收用关联负向分录或冲正，不删除原充值分录。退款导致可用余额不足的策略待产品决定。
- 账本单位需要明确：人民币最小单位、抽象点数，或两者分账；不把“模型 token”与“充值额度”混称。

## 5. 托管模型结算与故障恢复

- 每次客户端提交带稳定幂等键；服务端对 `account_id + idempotency_key` 建唯一约束并存请求摘要，重复请求返回原任务。
- 先按服务端目录与价格快照计算 `reservation`；上游请求由具有超时/取消/脱敏边界的 provider adapter 发出。
- 已知上游 receipt 时，恢复只查询原任务，不重复提交。上游结果不确定且供应商不支持查询/幂等时，进入 `submission_unknown`，暂停自动重发并进行对账/人工处理。
- 任务完成后按供应商计量或已确认规则结算；无可靠计量的模型只能使用明确的固定价格/上限，不得把未知成本按零结算。
- 写入 usage 和账本结算必须可重放且幂等；供应商失败、取消、部分成功及退款规则须按模型合同实现。

## 6. 安全、隐私与运营约束

- OAuth 的 state/临时码随机、短期、单次使用；回调精确验证已登记应用和 URL，不使用客户端提供的任意回调地址。
- 云会话需短期访问凭据与可撤销刷新机制；客户端存储使用平台安全能力，具体跨平台方案评估后定。服务端存储凭据时优先存哈希/轮换材料。
- 支付通知验签、金额/币种/商户/订单核对由服务端完成；通知重复、乱序和延迟都有状态处理。
- 上游密钥经云端秘密管理读取；日志、trace、诊断和错误信息不得包含密钥、完整支付凭据、OAuth code 或不必要个人数据。
- 模型代理应限制账号/模型/费用权限；用户提交的渠道、URL、header 不得进入托管上游凭据路径。托管 Provider 使用服务端固定配置，不开放任意 URL 代理。
- 所有运营性加额度、退款、冻结和账号封禁操作需强权限、理由、幂等保护及审计记录。
- 生产部署前完成威胁建模、备份恢复、账务对账、密钥轮换、监控告警和沙箱端到端验收。

## 7. 测试设计

- Identity 单测：state 匹配/错配、过期、取消、重复回调、重复兑换、身份绑定冲突、撤销。
- Payment/Ledger 集成测试：验签失败、金额不符、重复/乱序通知、并发同单、退款、事务回滚、账本重算。
- Gateway 集成测试：权限拒绝、额度不足、幂等重试、超时未知、上游失败、已 receipt 恢复、用量结算重复。
- API 授权测试：跨账号读取/取消订单和任务拒绝；客户端不能伪造目录价格、支付成功或实际用量。
- 桌面验收：未登录与云端断网仍能使用本地项目和 BYOK；退出后云能力拒绝访问，本地数据仍存在。
- 沙箱端到端：扫码、下单、支付通知、入账、模型生成、扣费、退款/对账。生产支付和真实模型验收另行审批。

## 8. 尚待设计决定

- 已确认云服务在本仓库 `backend/cmd/cloud` 与 `backend/internal/cloud` 内独立部署，拥有独立组合根、配置和云专属数据边界；沙箱使用独立 SQLite 文件，仅用于开发/测试。生产部署与数据存储产品仍待决策。
- 微信开放平台应用类型与授权回调详细协议；桌面端凭据安全存储方案。
- 支付宝签约产品和 SDK/验签组件；微信支付是否纳入首期。
- 额度计量单位、价格快照规则、退款余额不足策略和保留期限。
- 模型任务数据/生成内容是否经过云服务、存储期限和用户删除/导出机制。
- 是否需要 SSE、消息队列、对象存储；优先在确认真实并发和运维条件后决定，不为架构完整性预先引入。
