package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ecoclaim/internal/claim"
	"ecoclaim/internal/gateway"
	"ecoclaim/internal/model"
	"ecoclaim/internal/seed"
)

func newTestServer(t *testing.T, latency time.Duration) http.Handler {
	t.Helper()
	reg, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load 失败: %v", err)
	}
	gw := gateway.New(gateway.Options{Latency: latency})
	fixed := time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)
	srv := New(Options{
		Registry: reg,
		Claims:   claim.NewService(reg, gw),
		Now:      func() time.Time { return fixed },
	})
	return srv.Handler()
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var payload map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("%s %s 响应不是合法 JSON: %v\n%s", method, path, err, rec.Body.String())
		}
	}
	return rec, payload
}

func errorCode(t *testing.T, payload map[string]any) string {
	t.Helper()
	raw, ok := payload["error"]
	if !ok {
		t.Fatalf("响应缺少 error 字段: %+v", payload)
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("error 字段格式异常: %+v", raw)
	}
	code, _ := obj["code"].(string)
	return code
}

func TestHealth(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodGet, "/healthz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	if payload["status"] != "ok" {
		t.Fatalf("响应 = %+v", payload)
	}
}

func TestClaimNotFound(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodGet, "/api/claims/NOPE", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404", rec.Code)
	}
	if got := errorCode(t, payload); got != string(CodeClaimUnknown) {
		t.Fatalf("错误码 = %q", got)
	}
}

func TestClaimsList(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodGet, "/api/claims", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	if claims, _ := payload["claims"].([]any); len(claims) != len(seed.Claims()) {
		t.Fatalf("案件数 = %d, 期望 %d", len(claims), len(seed.Claims()))
	}

	rec, _ = doJSON(t, h, http.MethodGet, "/api/claims?stage=restoration", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	rec, payload = doJSON(t, h, http.MethodGet, "/api/claims?stage=nope", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法阶段状态码 = %d, 期望 400", rec.Code)
	}
	if got := errorCode(t, payload); got != string(CodeBadRequest) {
		t.Fatalf("错误码 = %q", got)
	}
}

// TestValidateEndpointOnCompleteClaim 断言材料齐备的案件校验接口返回 200 且 ok=true。
func TestValidateEndpointOnCompleteClaim(t *testing.T) {
	h := newTestServer(t, 0)
	// 先核定金额，让案件材料齐备。
	rec, payload := doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-001/assess", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("assess 状态码 = %d, 响应 %+v", rec.Code, payload)
	}
	rec, payload = doJSON(t, h, http.MethodGet, "/api/claims/HJ-2026-001/validate", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("材料齐备案件校验状态码 = %d, 期望 200, 响应 %+v", rec.Code, payload)
	}
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("ok = false, 响应 %+v", payload)
	}
}

// TestValidateEndpointOnIncompleteClaim 断言材料缺失的案件校验接口返回 422。
func TestValidateEndpointOnIncompleteClaim(t *testing.T) {
	h := newTestServer(t, 0)
	// HJ-2026-007 处于线索登记阶段，缺少多项材料。
	rec, payload := doJSON(t, h, http.MethodGet, "/api/claims/HJ-2026-007/validate", "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("状态码 = %d, 期望 422, 响应 %+v", rec.Code, payload)
	}
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("ok = true, 期望 false")
	}
}

// TestFinalizeEndpointRejectsIncompleteClaim 断言前置校验不通过时结案返回失败且阶段不变。
func TestFinalizeEndpointRejectsIncompleteClaim(t *testing.T) {
	h := newTestServer(t, 0)
	id := seed.RestorationClaimIDs()[0]

	_, before := doJSON(t, h, http.MethodGet, "/api/claims/"+id, "")
	beforeClaim, _ := before["claim"].(map[string]any)
	beforeStage, _ := beforeClaim["stage"].(string)

	rec, payload := doJSON(t, h, http.MethodPost, "/api/claims/"+id+"/finalize", `{"operator":"测试"}`)
	if rec.Code == http.StatusOK {
		t.Fatalf("未核定赔偿金额的案件结案不应返回 200, 响应 %+v", payload)
	}

	_, after := doJSON(t, h, http.MethodGet, "/api/claims/"+id, "")
	afterClaim, _ := after["claim"].(map[string]any)
	afterStage, _ := afterClaim["stage"].(string)
	if afterStage != beforeStage {
		t.Fatalf("结案失败时阶段不应变化: %s -> %s", beforeStage, afterStage)
	}
	if afterStage == string(model.StageClosed) {
		t.Fatalf("案件不应被标记为结案")
	}
}

// TestFinalizeEndpointSucceedsAfterAssess 断言核定金额后可以结案。
func TestFinalizeEndpointSucceedsAfterAssess(t *testing.T) {
	h := newTestServer(t, 0)
	id := seed.RestorationClaimIDs()[0]
	rec, payload := doJSON(t, h, http.MethodPost, "/api/claims/"+id+"/assess", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("assess 状态码 = %d, 响应 %+v", rec.Code, payload)
	}
	rec, payload = doJSON(t, h, http.MethodPost, "/api/claims/"+id+"/finalize", `{"operator":"测试"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("结案状态码 = %d, 期望 200, 响应 %+v", rec.Code, payload)
	}
	if finalized, _ := payload["finalized"].(bool); !finalized {
		t.Fatalf("finalized = false, 响应 %+v", payload)
	}
	if payload["stage"] != string(model.StageClosed) {
		t.Fatalf("stage = %v, 期望 closed", payload["stage"])
	}
}

// TestAssessEndpointHonoursTimeout 断言核定金额接口在超时后立即返回 503。
func TestAssessEndpointHonoursTimeout(t *testing.T) {
	h := newTestServer(t, 5*time.Second)
	begin := time.Now()
	rec, payload := doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-001/assess?timeout_ms=80", "")
	elapsed := time.Since(begin)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d, 期望 503, 响应 %+v", rec.Code, payload)
	}
	if got := errorCode(t, payload); got != string(CodeUnavailable) {
		t.Fatalf("错误码 = %q, 期望 %q", got, CodeUnavailable)
	}
	if elapsed > time.Second {
		t.Fatalf("耗时 = %v, 期望远小于 1s（外部耗时 5s）", elapsed)
	}
}

// TestProbeEndpointHonoursTimeout 断言网关探测接口在超时后立即返回。
func TestProbeEndpointHonoursTimeout(t *testing.T) {
	h := newTestServer(t, 5*time.Second)
	begin := time.Now()
	rec, payload := doJSON(t, h, http.MethodGet, "/api/gateway/probe?timeout_ms=60", "")
	elapsed := time.Since(begin)

	if rec.Code == http.StatusOK {
		t.Fatalf("超时应返回失败状态码, 响应 %+v", payload)
	}
	if elapsed > time.Second {
		t.Fatalf("耗时 = %v, 期望远小于 1s", elapsed)
	}
}

// TestSettleEndpointCoversEveryClaim 断言批量核算接口覆盖全部案件。
func TestSettleEndpointCoversEveryClaim(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodPost, "/api/docket/settle?workers=3&settle_ms=20", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 响应 %+v", rec.Code, payload)
	}
	requested, _ := payload["requested"].(float64)
	outcomes, _ := payload["outcomes"].(float64)
	if int(requested) != len(seed.Claims()) {
		t.Fatalf("requested = %v", payload["requested"])
	}
	if outcomes != requested {
		t.Fatalf("产出 %v 件, 请求 %v 件（必须覆盖全部案件）", outcomes, requested)
	}
	if complete, _ := payload["complete"].(bool); !complete {
		t.Fatalf("complete = false, 响应 %+v", payload)
	}
}

// TestScheduleEndpointNoticeDue 断言时限接口返回的告知期限为 20 个工作日之后。
func TestScheduleEndpointNoticeDue(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodGet, "/api/claims/HJ-2026-001/schedule", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	schedule, ok := payload["schedule"].(map[string]any)
	if !ok {
		t.Fatalf("schedule = %+v", payload["schedule"])
	}
	due, _ := schedule["notice_due"].(string)
	if !strings.HasPrefix(due, "2026-04-06") {
		t.Fatalf("notice_due = %v, 期望 2026-04-06", due)
	}
}

func TestAdvanceEndpoint(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-007/advance", `{"to":"investigation","operator":"测试"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 响应 %+v", rec.Code, payload)
	}
	if payload["stage"] != "investigation" {
		t.Fatalf("stage = %v", payload["stage"])
	}

	rec, payload = doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-007/advance", `{"to":"closed"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("非法流转状态码 = %d, 期望 409", rec.Code)
	}
	if got := errorCode(t, payload); got != string(CodeStageConflict) {
		t.Fatalf("错误码 = %q", got)
	}
}

func TestReportEndpoints(t *testing.T) {
	h := newTestServer(t, 0)
	rec, payload := doJSON(t, h, http.MethodGet, "/api/report/docket", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	if lines, _ := payload["lines"].([]any); len(lines) != len(seed.Claims()) {
		t.Fatalf("台账行数 = %d", len(lines))
	}

	rec, payload = doJSON(t, h, http.MethodGet, "/api/report/kinds", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	if kinds, _ := payload["kinds"].([]any); len(kinds) != 6 {
		t.Fatalf("损害类型行数 = %d, 期望 6", len(kinds))
	}

	rec, _ = doJSON(t, h, http.MethodGet, "/api/report/provinces", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
	rec, _ = doJSON(t, h, http.MethodGet, "/api/respondents", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d", rec.Code)
	}
}

func TestBadRequestParams(t *testing.T) {
	h := newTestServer(t, 0)
	if rec, _ := doJSON(t, h, http.MethodPost, "/api/docket/settle?workers=0", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("workers=0 状态码 = %d, 期望 400", rec.Code)
	}
	if rec, _ := doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-001/assess?timeout_ms=0", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("timeout_ms=0 状态码 = %d, 期望 400", rec.Code)
	}
	if rec, _ := doJSON(t, h, http.MethodPost, "/api/claims/HJ-2026-007/advance", `not-json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 状态码 = %d, 期望 400", rec.Code)
	}
	if rec, _ := doJSON(t, h, http.MethodGet, "/api/report/docket?at=2026/08/16", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法日期状态码 = %d, 期望 400", rec.Code)
	}
}

func TestClassifyMapsSentinels(t *testing.T) {
	cases := []struct {
		err        error
		wantStatus int
		wantCode   ErrorCode
	}{
		{fmt.Errorf("上层: %w", model.ErrClaimUnknown), http.StatusNotFound, CodeClaimUnknown},
		{fmt.Errorf("上层: %w", model.ErrStageConflict), http.StatusConflict, CodeStageConflict},
		{fmt.Errorf("上层: %w", model.ErrDeadlineExpired), http.StatusConflict, CodeDeadlineExpired},
		{fmt.Errorf("上层: %w", model.ErrValidationFailed), http.StatusUnprocessableEntity, CodeValidationFailed},
		{fmt.Errorf("上层: %w", model.ErrGatewayUnavailable), http.StatusBadGateway, CodeGatewayUnavailable},
		{fmt.Errorf("上层: %w", context.DeadlineExceeded), http.StatusServiceUnavailable, CodeUnavailable},
		{errors.New("未归类"), http.StatusInternalServerError, CodeInternal},
	}
	for _, tc := range cases {
		status, code := Classify(tc.err)
		if status != tc.wantStatus || code != tc.wantCode {
			t.Errorf("Classify(%v) = %d/%s, 期望 %d/%s", tc.err, status, code, tc.wantStatus, tc.wantCode)
		}
	}
}
