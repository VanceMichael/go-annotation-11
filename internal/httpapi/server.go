// Package httpapi 提供生态环境损害赔偿案件管理平台的 HTTP 接口。
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ecoclaim/internal/claim"
	"ecoclaim/internal/docket"
	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
	"ecoclaim/internal/report"
)

// ErrorCode 是对外暴露的机器可读错误码。
type ErrorCode string

const (
	// CodeClaimUnknown 案件不存在。
	CodeClaimUnknown ErrorCode = "claim_unknown"
	// CodeRespondentUnknown 赔偿义务人不存在。
	CodeRespondentUnknown ErrorCode = "respondent_unknown"
	// CodeDuplicateClaim 案件重复登记。
	CodeDuplicateClaim ErrorCode = "duplicate_claim"
	// CodeStageConflict 阶段流转冲突。
	CodeStageConflict ErrorCode = "stage_conflict"
	// CodeDeadlineExpired 法定时限届满。
	CodeDeadlineExpired ErrorCode = "deadline_expired"
	// CodeAssessmentIncomplete 鉴定评估未完成。
	CodeAssessmentIncomplete ErrorCode = "assessment_incomplete"
	// CodeValidationFailed 材料校验未通过。
	CodeValidationFailed ErrorCode = "validation_failed"
	// CodeGatewayUnavailable 鉴定评估网关不可用。
	CodeGatewayUnavailable ErrorCode = "gateway_unavailable"
	// CodeBadRequest 请求参数非法。
	CodeBadRequest ErrorCode = "bad_request"
	// CodeUnavailable 请求被取消或超时。
	CodeUnavailable ErrorCode = "unavailable"
	// CodeInternal 未归类的内部错误。
	CodeInternal ErrorCode = "internal"
)

var errorMapping = []struct {
	sentinel error
	status   int
	code     ErrorCode
}{
	{model.ErrClaimUnknown, http.StatusNotFound, CodeClaimUnknown},
	{model.ErrRespondentUnknown, http.StatusNotFound, CodeRespondentUnknown},
	{model.ErrDuplicateClaim, http.StatusConflict, CodeDuplicateClaim},
	{model.ErrStageConflict, http.StatusConflict, CodeStageConflict},
	{model.ErrDeadlineExpired, http.StatusConflict, CodeDeadlineExpired},
	{model.ErrAssessmentIncomplete, http.StatusConflict, CodeAssessmentIncomplete},
	{model.ErrValidationFailed, http.StatusUnprocessableEntity, CodeValidationFailed},
	{model.ErrGatewayUnavailable, http.StatusBadGateway, CodeGatewayUnavailable},
	{model.ErrInvalidClaim, http.StatusBadRequest, CodeBadRequest},
	{model.ErrUnknownDamageKind, http.StatusBadRequest, CodeBadRequest},
	{model.ErrUnknownSeverity, http.StatusBadRequest, CodeBadRequest},
	{model.ErrUnknownStage, http.StatusBadRequest, CodeBadRequest},
	{context.Canceled, http.StatusServiceUnavailable, CodeUnavailable},
	{context.DeadlineExceeded, http.StatusServiceUnavailable, CodeUnavailable},
}

// Classify 依据错误链把领域错误映射为 HTTP 状态码与错误码。
func Classify(err error) (int, ErrorCode) {
	if err == nil {
		return http.StatusOK, ""
	}
	for _, m := range errorMapping {
		if errors.Is(err, m.sentinel) {
			return m.status, m.code
		}
	}
	return http.StatusInternalServerError, CodeInternal
}

// Server 是平台 HTTP 服务。
type Server struct {
	registry *registry.Registry
	claims   *claim.Service
	reports  *report.Builder
	now      func() time.Time
}

// Options 是构造 Server 所需的依赖。
type Options struct {
	Registry *registry.Registry
	Claims   *claim.Service
	Now      func() time.Time
}

// New 构造 HTTP 服务。
func New(opts Options) *Server {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Server{
		registry: opts.Registry,
		claims:   opts.Claims,
		reports:  report.NewBuilder(opts.Registry, claim.MissingCount),
		now:      now,
	}
}

// Handler 返回注册好全部路由的处理器。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/claims", s.handleClaims)
	mux.HandleFunc("GET /api/claims/{id}", s.handleClaim)
	mux.HandleFunc("GET /api/claims/{id}/schedule", s.handleSchedule)
	mux.HandleFunc("GET /api/claims/{id}/validate", s.handleValidate)
	mux.HandleFunc("POST /api/claims/{id}/assess", s.handleAssess)
	mux.HandleFunc("POST /api/claims/{id}/advance", s.handleAdvance)
	mux.HandleFunc("POST /api/claims/{id}/finalize", s.handleFinalize)
	mux.HandleFunc("GET /api/respondents", s.handleRespondents)
	mux.HandleFunc("GET /api/report/docket", s.handleDocket)
	mux.HandleFunc("GET /api/report/provinces", s.handleProvinces)
	mux.HandleFunc("GET /api/report/kinds", s.handleKinds)
	mux.HandleFunc("POST /api/docket/settle", s.handleSettle)
	mux.HandleFunc("GET /api/gateway/probe", s.handleProbe)
	return mux
}

type errorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, err error) {
	status, code := Classify(err)
	s.writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: err.Error()}})
}

func (s *Server) badRequest(w http.ResponseWriter, msg string) {
	s.writeJSON(w, http.StatusBadRequest, errorEnvelope{Error: errorBody{Code: CodeBadRequest, Message: msg}})
}

func (s *Server) parseAt(r *http.Request) (time.Time, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("at"))
	if raw == "" {
		return s.now(), nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	c := s.registry.Counts()
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"service":     "ecoclaim",
		"claims":      c.Claims,
		"respondents": c.Respondents,
		"open":        c.Open,
		"closed":      c.Closed,
	})
}

func (s *Server) handleClaims(w http.ResponseWriter, r *http.Request) {
	if raw := strings.TrimSpace(r.URL.Query().Get("stage")); raw != "" {
		stage, err := model.ParseStage(raw)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"claims": s.registry.ClaimsByStage(stage)})
		return
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("kind")); raw != "" {
		kind, err := model.ParseDamageKind(raw)
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{"claims": s.registry.ClaimsByKind(kind)})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"claims": s.registry.Claims()})
}

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	c, err := s.registry.Claim(r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"claim":         c,
		"stage_name":    c.Stage.DisplayName(),
		"kind_name":     c.Kind.DisplayName(),
		"allowed_next":  claim.Allowed(c.Stage),
		"missing_items": claim.MissingCount(c),
	})
}

func (s *Server) handleSchedule(w http.ResponseWriter, r *http.Request) {
	schedule, err := s.claims.Schedule(r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	at, perr := s.parseAt(r)
	if perr != nil {
		s.badRequest(w, "httpapi: at 需为 RFC3339 时间")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"schedule":            schedule,
		"notice_overdue":      schedule.NoticeOverdue(at),
		"assessment_overdue":  schedule.AssessmentOverdue(at),
		"negotiation_overdue": schedule.NegotiationOverdue(at),
		"litigation_barred":   schedule.LitigationBarred(at),
		"remaining_workdays":  schedule.RemainingWorkdaysForNotice(at),
	})
}

// handleValidate 返回案件结案材料校验结果。
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	c, err := s.registry.Claim(r.PathValue("id"))
	if err != nil {
		s.writeError(w, err)
		return
	}
	if verr := claim.Validate(c); verr != nil {
		s.writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"claim_id": c.ID,
			"ok":       false,
			"error":    errorBody{Code: CodeValidationFailed, Message: verr.Error()},
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"claim_id": c.ID, "ok": true})
}

func (s *Server) handleAssess(w http.ResponseWriter, r *http.Request) {
	timeout := 5 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout_ms")); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms <= 0 {
			s.badRequest(w, "httpapi: timeout_ms 需为正整数")
			return
		}
		timeout = time.Duration(ms) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	begin := time.Now()
	breakdown, err := s.claims.Assess(ctx, r.PathValue("id"))
	elapsed := time.Since(begin)
	if err != nil {
		status, code := Classify(err)
		s.writeJSON(w, status, map[string]any{
			"error":      errorBody{Code: code, Message: err.Error()},
			"elapsed_ms": elapsed.Milliseconds(),
			"timeout_ms": timeout.Milliseconds(),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"breakdown":  breakdown,
		"elapsed_ms": elapsed.Milliseconds(),
		"timeout_ms": timeout.Milliseconds(),
	})
}

type advanceRequest struct {
	To       string `json:"to"`
	Operator string `json:"operator"`
	Note     string `json:"note"`
	At       string `json:"at"`
}

func (s *Server) handleAdvance(w http.ResponseWriter, r *http.Request) {
	var body advanceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.badRequest(w, "httpapi: 请求体不是合法 JSON")
		return
	}
	to, err := model.ParseStage(body.To)
	if err != nil {
		s.writeError(w, err)
		return
	}
	at := s.now()
	if strings.TrimSpace(body.At) != "" {
		parsed, perr := time.Parse(time.RFC3339, body.At)
		if perr != nil {
			s.badRequest(w, "httpapi: at 需为 RFC3339 时间")
			return
		}
		at = parsed
	}
	next, err := s.claims.Advance(r.PathValue("id"), to, body.Operator, body.Note, at)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, next)
}

type finalizeRequest struct {
	Operator string `json:"operator"`
	At       string `json:"at"`
}

// handleFinalize 结案。前置校验不通过时必须返回失败状态码，
// 且案件阶段保持不变。
func (s *Server) handleFinalize(w http.ResponseWriter, r *http.Request) {
	var body finalizeRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	at := s.now()
	if strings.TrimSpace(body.At) != "" {
		parsed, perr := time.Parse(time.RFC3339, body.At)
		if perr != nil {
			s.badRequest(w, "httpapi: at 需为 RFC3339 时间")
			return
		}
		at = parsed
	}
	id := r.PathValue("id")
	operator := body.Operator
	if operator == "" {
		operator = "赔偿权利人指定部门"
	}

	if err := s.claims.Finalize(id, operator, at); err != nil {
		s.writeError(w, err)
		return
	}
	c, err := s.registry.Claim(id)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"claim_id":   c.ID,
		"stage":      string(c.Stage),
		"stage_name": c.Stage.DisplayName(),
		"finalized":  c.Stage == model.StageClosed,
	})
}

func (s *Server) handleRespondents(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"respondents": s.registry.Respondents()})
}

func (s *Server) handleDocket(w http.ResponseWriter, r *http.Request) {
	at, err := s.parseAt(r)
	if err != nil {
		s.badRequest(w, "httpapi: at 需为 RFC3339 时间")
		return
	}
	rep, derr := s.reports.Docket(at)
	if derr != nil {
		s.writeError(w, derr)
		return
	}
	s.writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleProvinces(w http.ResponseWriter, r *http.Request) {
	lines, err := s.reports.Provinces()
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"provinces": lines})
}

func (s *Server) handleKinds(w http.ResponseWriter, r *http.Request) {
	lines, err := s.reports.Kinds()
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"kinds": lines})
}

func (s *Server) handleSettle(w http.ResponseWriter, r *http.Request) {
	workers := 4
	if raw := strings.TrimSpace(r.URL.Query().Get("workers")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			s.badRequest(w, "httpapi: workers 需为正整数")
			return
		}
		workers = n
	}
	var settle time.Duration
	if raw := strings.TrimSpace(r.URL.Query().Get("settle_ms")); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms < 0 {
			s.badRequest(w, "httpapi: settle_ms 需为非负整数")
			return
		}
		settle = time.Duration(ms) * time.Millisecond
	}

	runner := docket.New(workers, settle)
	res, err := runner.Run(r.Context(), docket.ItemsFrom(s.registry.Claims()))
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"requested":  res.Requested,
		"completed":  res.Completed,
		"failed":     res.Failed,
		"complete":   res.Complete(),
		"outcomes":   len(res.Outcomes),
		"summary":    res.Summary,
		"workers":    res.Workers,
		"elapsed_ms": res.Elapsed.Milliseconds(),
	})
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	timeout := 2 * time.Second
	if raw := strings.TrimSpace(r.URL.Query().Get("timeout_ms")); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms <= 0 {
			s.badRequest(w, "httpapi: timeout_ms 需为正整数")
			return
		}
		timeout = time.Duration(ms) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	begin := time.Now()
	err := s.claims.Probe(ctx)
	elapsed := time.Since(begin)
	if err != nil {
		status, code := Classify(err)
		s.writeJSON(w, status, map[string]any{
			"error":      errorBody{Code: code, Message: err.Error()},
			"elapsed_ms": elapsed.Milliseconds(),
			"timeout_ms": timeout.Milliseconds(),
		})
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"elapsed_ms": elapsed.Milliseconds(),
		"timeout_ms": timeout.Milliseconds(),
	})
}
