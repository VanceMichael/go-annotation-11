# Bug Reproduction

## 包的性质

当前 test_model_fix 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。要复现原始缺陷，必须检出下面固定的 parent SHA；不要在当前修复结果源码上期待重新出现修复前失败。生成系统使用的可信验证补丁和完整验证日志仅在本地留存，不提交到结果分支。

## 问题现象

结案接口报成功，但案件其实没结掉，台账里对不上。

```
$ ./ecoctl claim finalize --id HJ-2026-001
{
  "claim_id": "HJ-2026-001",
  "stage_before": "restoration",
  "stage_after": "restoration",
  "finalized": false,
  "ok": true
}
$ echo $?
0
```

HJ-2026-001 还没核定赔偿金额，按约定结案前置校验就该拦下来，返回错误、退出码非 0。实际是 ok=true、退出码 0，但 stage 从头到尾都是 restoration，`finalized` 也是 false —— 返回值说成功，数据说没做，两边打脸。

HTTP 那边同样：POST /api/claims/HJ-2026-001/finalize 返回 200，响应里 finalized 是 false。

对照一下：案件编号不存在时会正常报 404 / 退出码 5；先跑一遍 assess 把金额核定好，再结案是能真正结掉的。所以坏的只是「前置校验不通过」这条路。

帮我修一下，让前置校验失败时如实返回错误。已有测试跑一遍不要有回归。

## 含 Bug 版本

- 仓库：VanceMichael/go-annotation-11
- 仓库地址：https://github.com/VanceMichael/go-annotation-11.git
- parent SHA：d12f61434916ed1f458d0bf038f79c75d546eaf2

## 复现步骤

```bash
git clone -- https://github.com/VanceMichael/go-annotation-11.git bug-repro
cd bug-repro
git checkout --detach d12f61434916ed1f458d0bf038f79c75d546eaf2
go test ./internal/claim/ -run "TestFinalizeReportsPrecheckFailure|TestFinalizeSurfacesValidationSentinel|TestFinalizeSucceedsWhenReady" -count=1
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/claim/ -run "TestFinalizeReportsPrecheckFailure|TestFinalizeSurfacesValidationSentinel|TestFinalizeSucceedsWhenReady" -count=1
--- FAIL: TestFinalizeReportsPrecheckFailure (0.00s)
    claim_test.go:130: 未核定赔偿金额的案件结案应返回错误, 实际返回 nil
--- FAIL: TestFinalizeSurfacesValidationSentinel (0.00s)
    claim_test.go:161: 材料缺失的案件结案应返回错误
FAIL
FAIL	ecoclaim/internal/claim	0.032s
FAIL

```

stderr：

```text
(empty)
```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test ./internal/claim/ -run "TestFinalizeReportsPrecheckFailure|TestFinalizeSurfacesValidationSentinel|TestFinalizeSucceedsWhenReady" -count=1
--- FAIL: TestFinalizeReportsPrecheckFailure (0.00s)
    claim_test.go:130: 未核定赔偿金额的案件结案应返回错误, 实际返回 nil
--- FAIL: TestFinalizeSurfacesValidationSentinel (0.00s)
    claim_test.go:161: 材料缺失的案件结案应返回错误
FAIL
FAIL	ecoclaim/internal/claim	0.002s
FAIL

```

stderr：

```text
(empty)
```

## 通过条件

定向测试与全量回归在 linux/amd64、linux/arm64 双架构下均通过。
前置校验不通过时结案返回非 nil 错误，案件阶段保持不变，且不得被标记为结案。
材料缺失导致的失败可通过 errors.Is 判定为 model.ErrValidationFailed；CLI 退出码与 HTTP 状态码按约定分类。
材料齐备且已核定金额的案件仍能正常结案，并追加一条阶段流转记录。
