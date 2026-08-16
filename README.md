# ecoclaim —— 生态环境损害赔偿案件管理平台

`ecoclaim` 是一个纯 Go 实现的后端与命令行工具，面向生态环境损害赔偿制度改革实践，
覆盖案件线索登记、损害类型认定、法定时限管理、赔偿金额核算、外部鉴定评估调用、
批量核算与结案归档。

项目不依赖任何第三方模块，只使用 Go 标准库。

## 业务背景

### 法定时限

平台涉及两类时限口径：

- **工作日口径**：跳过周六、周日。**起始日本身不计入**，即立案当日不算第一个工作日。
- **自然日口径**：直接按日历天数推进。

| 环节 | 时限 | 口径 |
| --- | --- | --- |
| 磋商启动告知 | 立案后 20 个工作日 | 工作日 |
| 鉴定评估报告提交 | 进入评估阶段后 60 日 | 自然日 |
| 磋商期限 | 磋商启动后 90 日 | 自然日 |
| 诉讼时效 | 损害发现之日起 3 年 | 自然年 |

### 赔偿金额核算

```
赔偿金额 = (清污费用 + 修复费用 + 期间损害 + 基准费用 + 调查鉴定费用) × 损害程度系数
```

| 费率项 | 取值 |
| --- | --- |
| 每亩清污费用 | 1800 元 |
| 每亩原地修复费用 | 4200 元 |
| 每亩替代修复费用 | 6800 元 |
| 每亩每日期间损害 | 12.5 元 |
| 调查鉴定费用 | 直接费用的 6%，下限 20000 元 |

损害程度系数：轻微 1.0、较重 1.5、严重 2.2、特别严重 3.0。
不可原地修复的案件按替代修复计价，费率上浮。结果四舍五入到分。

### 案件阶段流转

```
intake ──> investigation ──> assessment ──> negotiation ──┬──> agreement ──> restoration ──> closed
                                                          └──> litigation ──> restoration ──> closed
```

任一非终态阶段均可流转到 `terminated`。结案（`closed`）要求前置校验通过：
材料齐备、诉讼时效未届满、赔偿金额已核定。

## 目录结构

```
cmd/ecoctl            命令行入口
internal/model        领域模型与哨兵错误
internal/deadline     法定时限计算
internal/assess       赔偿金额核算
internal/gateway      外部鉴定评估机构调用
internal/registry     案件与赔偿义务人存储
internal/claim        阶段流转、材料校验与结案服务
internal/docket       案件批量核算
internal/report       台账、省份与损害类型报表
internal/httpapi      HTTP 接口
internal/seed         内置样例数据
internal/cli          ecoctl 命令实现
```

## 构建与测试

```bash
export GOTOOLCHAIN=local

go build ./...
go test ./...
go test -race ./...

make build           # 产出 bin/ecoctl
make selfcheck       # 构建并运行内置自检
```

## 命令行用法

```bash
ecoctl claim list
ecoctl claim list --stage restoration
ecoctl claim show --id HJ-2026-001
ecoctl claim validate --id HJ-2026-001
ecoctl claim advance --id HJ-2026-007 --to investigation
ecoctl claim assess --id HJ-2026-001
ecoctl claim assess --id HJ-2026-001 --timeout 200ms --gateway-latency 5s
ecoctl claim finalize --id HJ-2026-001

ecoctl deadline show --id HJ-2026-001
ecoctl docket settle --workers 4 --settle 20ms

ecoctl report docket
ecoctl report provinces
ecoctl report kinds

ecoctl gateway probe --timeout 2s
ecoctl serve --addr 127.0.0.1:8080
ecoctl selfcheck
```

### 退出码

| 退出码 | 含义 |
| --- | --- |
| 0 | 成功 |
| 1 | 用法错误或未归类的内部错误 |
| 2 | 参数非法 |
| 3 | 业务冲突（阶段流转冲突、时限届满、鉴定未完成等） |
| 4 | 外部调用被取消或超时 |
| 5 | 资源不存在 |
| 6 | 案件材料校验未通过 |

## HTTP 接口

```
GET  /healthz
GET  /api/claims[?stage=restoration|?kind=soil]
GET  /api/claims/{id}
GET  /api/claims/{id}/schedule[?at=RFC3339]
GET  /api/claims/{id}/validate
POST /api/claims/{id}/assess[?timeout_ms=5000]
POST /api/claims/{id}/advance
POST /api/claims/{id}/finalize
GET  /api/respondents
GET  /api/report/docket[?at=RFC3339]
GET  /api/report/provinces
GET  /api/report/kinds
POST /api/docket/settle[?workers=4&settle_ms=20]
GET  /api/gateway/probe[?timeout_ms=2000]
```

错误响应统一为：

```json
{ "error": { "code": "stage_conflict", "message": "..." } }
```

状态码约定：`404` 资源不存在，`409` 业务冲突，`422` 材料校验未通过，
`400` 参数非法，`502` 鉴定评估网关不可用，`503` 调用被取消或超时，
`500` 未归类的内部错误。

> 说明：HTTP 接口默认不带鉴权，仅面向内网或本地演练环境。若需暴露到公网，
> 必须在前置网关补充身份认证与访问控制。

## 关键行为约定

- **材料校验**：`claim.Validate` 在材料齐备时必须返回 nil，调用方以 `err != nil`
  判定校验结果；材料缺失时返回 `*ValidationError`，可用 `errors.Is` 判定为
  `model.ErrValidationFailed`。
- **结案**：前置校验不通过必须返回对应错误，且案件阶段保持不变。
- **外部调用**：鉴定评估是外部同步调用，每次调用都必须携带调用方传入的 context，
  调用方的超时或取消必须能够立即终止在途请求。
- **批量核算**：返回前必须等待所有工作协程结束，结果条数必须等于传入的案件件数。

## 容器运行

```bash
docker build -t ecoclaim:local .
docker run --rm ecoclaim:local selfcheck
docker run --rm -p 8080:8080 ecoclaim:local serve --addr 0.0.0.0:8080
```

镜像基于 `golang:1.22` 构建、`distroless/static` 运行，同时支持
`linux/amd64` 与 `linux/arm64`：

```bash
docker build --platform linux/amd64 -t ecoclaim:amd64 .
docker build --platform linux/arm64 -t ecoclaim:arm64 .
```

## 数据来源

内置样例数据（`internal/seed`）包含 6 个赔偿义务人与 7 件案件，覆盖土壤、水、
大气、噪声、固废与生态破坏六类损害，仅用于本地演练，不代表真实案件数据。
