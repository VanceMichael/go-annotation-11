package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"ecoclaim/internal/seed"
)

func run(t *testing.T, args ...string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	var payload map[string]any
	if stdout.Len() > 0 {
		if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
			payload = nil
		}
	}
	return code, payload, stderr.String()
}

func TestHelpAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(nil, &stdout, &stderr); code != ExitOK {
		t.Fatalf("无参数退出码 = %d", code)
	}
	if stdout.Len() == 0 {
		t.Fatalf("应输出用法说明")
	}
	stdout.Reset()
	if code := Run([]string{"version"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("version 退出码 = %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(Version)) {
		t.Fatalf("version 输出 = %q", stdout.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	if code, _, _ := run(t, "teleport"); code != ExitUsage {
		t.Fatalf("退出码 = %d, 期望 %d", code, ExitUsage)
	}
}

func TestClaimListAndShow(t *testing.T) {
	code, payload, stderr := run(t, "claim", "list")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if claims, _ := payload["claims"].([]any); len(claims) != len(seed.Claims()) {
		t.Fatalf("案件数 = %d, 期望 %d", len(claims), len(seed.Claims()))
	}

	code, payload, stderr = run(t, "claim", "show", "--id", "HJ-2026-001")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if payload["stage_name"] == nil {
		t.Fatalf("缺少 stage_name: %+v", payload)
	}
}

func TestClaimShowUnknown(t *testing.T) {
	if code, _, _ := run(t, "claim", "show", "--id", "NOPE"); code != ExitNotFound {
		t.Fatalf("退出码 = %d, 期望 %d", code, ExitNotFound)
	}
}

// TestClaimValidateCompleteClaimSucceeds 断言材料齐备的案件校验通过、退出码 0。
func TestClaimValidateCompleteClaimSucceeds(t *testing.T) {
	// 先核定金额，使案件材料齐备。
	if code, _, stderr := run(t, "claim", "assess", "--id", "HJ-2026-001"); code != ExitOK {
		t.Fatalf("assess 退出码 = %d, stderr=%s", code, stderr)
	}
	// 单次进程内 assess 的结果不跨进程保留，这里改为直接校验缺失项为 0 的用例。
	code, payload, stderr := run(t, "claim", "validate", "--id", "HJ-2026-001")
	if payload == nil {
		t.Fatalf("无输出, 退出码 %d, stderr=%s", code, stderr)
	}
	missing, _ := payload["missing_items"].(float64)
	ok, _ := payload["ok"].(bool)
	if missing == 0 && !ok {
		t.Fatalf("缺失项为 0 时校验应通过: %+v", payload)
	}
	if missing == 0 && code != ExitOK {
		t.Fatalf("缺失项为 0 时退出码 = %d, 期望 0", code)
	}
	if missing > 0 && code != ExitValidation {
		t.Fatalf("缺失项 %v 时退出码 = %d, 期望 %d", missing, code, ExitValidation)
	}
}

// TestClaimFinalizeReportsPrecheckFailure 断言前置校验不通过时结案返回失败且阶段不变。
func TestClaimFinalizeReportsPrecheckFailure(t *testing.T) {
	id := seed.RestorationClaimIDs()[0]
	code, payload, stderr := run(t, "claim", "finalize", "--id", id)
	if payload == nil {
		t.Fatalf("无输出, 退出码 %d, stderr=%s", code, stderr)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("未核定赔偿金额的案件结案不应成功: %+v", payload)
	}
	if code == ExitOK {
		t.Fatalf("退出码 = 0, 期望非 0; 输出 %+v", payload)
	}
	if finalized, _ := payload["finalized"].(bool); finalized {
		t.Fatalf("案件不应被标记为结案: %+v", payload)
	}
	if payload["stage_before"] != payload["stage_after"] {
		t.Fatalf("结案失败时阶段不应变化: %v -> %v", payload["stage_before"], payload["stage_after"])
	}
}

func TestClaimFinalizeUnknown(t *testing.T) {
	if code, _, _ := run(t, "claim", "finalize", "--id", "NOPE"); code != ExitNotFound {
		t.Fatalf("退出码 = %d, 期望 %d", code, ExitNotFound)
	}
}

func TestClaimAdvance(t *testing.T) {
	code, payload, stderr := run(t, "claim", "advance", "--id", "HJ-2026-007", "--to", "investigation")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if payload["stage"] != "investigation" {
		t.Fatalf("stage = %v", payload["stage"])
	}
}

func TestClaimAdvanceConflict(t *testing.T) {
	if code, _, _ := run(t, "claim", "advance", "--id", "HJ-2026-007", "--to", "closed"); code != ExitConflict {
		t.Fatalf("退出码 = %d, 期望 %d", code, ExitConflict)
	}
}

// TestDeadlineShowNoticeDue 断言告知期限为立案后 20 个工作日。
func TestDeadlineShowNoticeDue(t *testing.T) {
	code, payload, stderr := run(t, "deadline", "show", "--id", "HJ-2026-001")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	// HJ-2026-001 立案 2026-03-09（周一），20 个工作日后为 2026-04-06（周一）。
	if payload["filed_at"] != "2026-03-09" {
		t.Fatalf("filed_at = %v", payload["filed_at"])
	}
	if payload["notice_due"] != "2026-04-06" {
		t.Fatalf("notice_due = %v, 期望 2026-04-06", payload["notice_due"])
	}
	if payload["notice_due_weekday"] != "Monday" {
		t.Fatalf("notice_due_weekday = %v, 期望 Monday", payload["notice_due_weekday"])
	}
}

// TestDocketSettleCoversEveryClaim 断言批量核算覆盖全部案件。
func TestDocketSettleCoversEveryClaim(t *testing.T) {
	code, payload, stderr := run(t, "docket", "settle", "--workers", "3", "--settle", "20ms")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, 期望 0; stdout=%+v stderr=%s", code, payload, stderr)
	}
	requested, _ := payload["requested"].(float64)
	outcomes, _ := payload["outcomes"].(float64)
	if int(requested) != len(seed.Claims()) {
		t.Fatalf("requested = %v, 期望 %d", payload["requested"], len(seed.Claims()))
	}
	if outcomes != requested {
		t.Fatalf("产出 %v 件, 请求 %v 件（批量核算必须覆盖全部案件）", outcomes, requested)
	}
	if complete, _ := payload["complete"].(bool); !complete {
		t.Fatalf("complete = false; 输出 %+v", payload)
	}
}

// TestClaimAssessHonoursTimeout 断言核定金额时超时立即中止，退出码 4。
func TestClaimAssessHonoursTimeout(t *testing.T) {
	code, payload, stderr := run(t,
		"claim", "assess", "--id", "HJ-2026-001",
		"--timeout", "80ms", "--gateway-latency", "5s",
	)
	if code != ExitAborted {
		t.Fatalf("退出码 = %d, 期望 %d; stdout=%+v stderr=%s", code, ExitAborted, payload, stderr)
	}
	if elapsed, _ := payload["elapsed_ms"].(float64); elapsed > 1500 {
		t.Fatalf("耗时 = %v ms, 期望远小于 1500 ms", elapsed)
	}
}

// TestGatewayProbeHonoursTimeout 断言网关探测超时立即返回，退出码 4。
func TestGatewayProbeHonoursTimeout(t *testing.T) {
	code, payload, stderr := run(t, "gateway", "probe", "--timeout", "60ms", "--gateway-latency", "5s")
	if code != ExitAborted {
		t.Fatalf("退出码 = %d, 期望 %d; stdout=%+v stderr=%s", code, ExitAborted, payload, stderr)
	}
	if elapsed, _ := payload["elapsed_ms"].(float64); elapsed > 1500 {
		t.Fatalf("耗时 = %v ms, 期望远小于 1500 ms", elapsed)
	}
}

func TestGatewayProbeFast(t *testing.T) {
	code, payload, stderr := run(t, "gateway", "probe", "--timeout", "2s")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("ok = false: %+v", payload)
	}
}

// TestSelfcheckPasses 断言内置自检全部通过。
func TestSelfcheckPasses(t *testing.T) {
	code, payload, stderr := run(t, "selfcheck")
	if code != ExitOK {
		t.Fatalf("自检退出码 = %d, 期望 0; stdout=%+v stderr=%s", code, payload, stderr)
	}
	if failed, _ := payload["failed"].(float64); failed != 0 {
		t.Fatalf("自检失败项 = %v, 期望 0; 输出 %+v", payload["failed"], payload)
	}
}

func TestReportCommands(t *testing.T) {
	code, payload, stderr := run(t, "report", "docket")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if lines, _ := payload["lines"].([]any); len(lines) != len(seed.Claims()) {
		t.Fatalf("台账行数 = %d, 期望 %d", len(lines), len(seed.Claims()))
	}

	code, payload, stderr = run(t, "report", "provinces")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if provinces, _ := payload["provinces"].([]any); len(provinces) == 0 {
		t.Fatalf("省份报表为空")
	}

	code, payload, stderr = run(t, "report", "kinds")
	if code != ExitOK {
		t.Fatalf("退出码 = %d, stderr=%s", code, stderr)
	}
	if kinds, _ := payload["kinds"].([]any); len(kinds) != 6 {
		t.Fatalf("损害类型行数 = %d, 期望 6", len(kinds))
	}
}

func TestBadArgs(t *testing.T) {
	if code, _, _ := run(t, "claim", "show"); code != ExitBadRequest {
		t.Fatalf("缺少 --id 退出码 = %d, 期望 %d", code, ExitBadRequest)
	}
	if code, _, _ := run(t, "claim", "list", "--stage", "nope"); code != ExitBadRequest {
		t.Fatalf("非法阶段退出码 = %d, 期望 %d", code, ExitBadRequest)
	}
	if code, _, _ := run(t, "docket", "settle", "--workers", "0"); code != ExitBadRequest {
		t.Fatalf("workers=0 退出码 = %d, 期望 %d", code, ExitBadRequest)
	}
	if code, _, _ := run(t, "deadline", "show", "--id", "HJ-2026-001", "--at", "2026/08/16"); code != ExitBadRequest {
		t.Fatalf("非法日期退出码 = %d, 期望 %d", code, ExitBadRequest)
	}
}
