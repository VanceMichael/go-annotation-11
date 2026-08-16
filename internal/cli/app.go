// Package cli 实现 ecoctl 命令行界面。
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"ecoclaim/internal/assess"
	"ecoclaim/internal/claim"
	"ecoclaim/internal/deadline"
	"ecoclaim/internal/docket"
	"ecoclaim/internal/gateway"
	"ecoclaim/internal/httpapi"
	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
	"ecoclaim/internal/report"
	"ecoclaim/internal/seed"
)

// 退出码约定。上层脚本依赖这些取值区分失败类别。
const (
	// ExitOK 正常结束。
	ExitOK = 0
	// ExitUsage 命令行用法错误或未归类的内部错误。
	ExitUsage = 1
	// ExitBadRequest 参数非法。
	ExitBadRequest = 2
	// ExitConflict 业务冲突：阶段流转冲突、时限届满、鉴定未完成等。
	ExitConflict = 3
	// ExitAborted 外部调用被取消或超时。
	ExitAborted = 4
	// ExitNotFound 资源不存在。
	ExitNotFound = 5
	// ExitValidation 案件材料校验未通过。
	ExitValidation = 6
)

// Version 是当前构建版本。
const Version = "0.2.0"

const usage = `ecoctl —— 生态环境损害赔偿案件管理平台命令行

用法:
  ecoctl <命令> [子命令] [参数]

命令:
  claim list         列出案件
  claim show         查看案件详情与可流转阶段
  claim validate     校验案件结案材料
  claim advance      推进案件阶段
  claim finalize     结案
  claim assess       核定赔偿金额（调用外部鉴定评估网关）
  deadline show      输出案件法定时限表
  docket settle      批量核算全部案件
  report docket      生成案件台账报表
  report provinces   生成省份维度报表
  report kinds       生成损害类型维度报表
  gateway probe      探测鉴定评估网关可用性
  serve              启动 HTTP 服务
  selfcheck          运行内置自检
  version            输出版本信息

退出码:
  0 成功  1 用法或内部错误  2 参数非法  3 业务冲突
  4 外部调用中止  5 资源不存在  6 材料校验未通过
`

type app struct {
	registry *registry.Registry
	claims   *claim.Service
	reports  *report.Builder
	gateway  *gateway.Client
	stdout   io.Writer
	stderr   io.Writer
}

func newApp(stdout, stderr io.Writer, latency time.Duration) (*app, error) {
	reg, err := seed.Load()
	if err != nil {
		return nil, err
	}
	gw := gateway.New(gateway.Options{Latency: latency})
	svc := claim.NewService(reg, gw)
	return &app{
		registry: reg,
		claims:   svc,
		reports:  report.NewBuilder(reg, claim.MissingCount),
		gateway:  gw,
		stdout:   stdout,
		stderr:   stderr,
	}, nil
}

// Run 执行一次命令行调用并返回退出码。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	code, err := route(args, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "错误: %v\n", err)
	}
	return code
}

func route(args []string, stdout, stderr io.Writer) (int, error) {
	// 先解出可能影响初始化的全局参数。
	latency := parseLatency(args)
	a, err := newApp(stdout, stderr, latency)
	if err != nil {
		fmt.Fprintf(stderr, "初始化失败: %v\n", err)
		return ExitUsage, nil
	}

	switch args[0] {
	case "version":
		fmt.Fprintf(stdout, "ecoctl %s\n", Version)
		return ExitOK, nil
	case "claim":
		return a.runClaim(args[1:])
	case "deadline":
		return a.runDeadline(args[1:])
	case "docket":
		return a.runDocket(args[1:])
	case "report":
		return a.runReport(args[1:])
	case "gateway":
		return a.runGateway(args[1:])
	case "serve":
		return a.runServe(args[1:])
	case "selfcheck":
		return a.runSelfcheck(args[1:])
	default:
		fmt.Fprint(stderr, usage)
		return ExitUsage, fmt.Errorf("未知命令 %q", args[0])
	}
}

// parseLatency 从参数中提取 --gateway-latency，供网关客户端初始化。
func parseLatency(args []string) time.Duration {
	for i := 0; i < len(args); i++ {
		if args[i] == "--gateway-latency" && i+1 < len(args) {
			if d, err := time.ParseDuration(args[i+1]); err == nil {
				return d
			}
		}
	}
	return 0
}

func (a *app) emit(payload any) error {
	enc := json.NewEncoder(a.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// classify 把领域错误映射为退出码，映射依据是错误链中的哨兵错误。
func classify(err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return ExitAborted
	case errors.Is(err, model.ErrGatewayUnavailable):
		return ExitAborted
	case errors.Is(err, model.ErrValidationFailed):
		return ExitValidation
	case errors.Is(err, model.ErrStageConflict),
		errors.Is(err, model.ErrDeadlineExpired),
		errors.Is(err, model.ErrAssessmentIncomplete),
		errors.Is(err, model.ErrDuplicateClaim):
		return ExitConflict
	case errors.Is(err, model.ErrClaimUnknown),
		errors.Is(err, model.ErrRespondentUnknown):
		return ExitNotFound
	case errors.Is(err, model.ErrInvalidClaim),
		errors.Is(err, model.ErrUnknownDamageKind),
		errors.Is(err, model.ErrUnknownSeverity),
		errors.Is(err, model.ErrUnknownStage):
		return ExitBadRequest
	default:
		return ExitUsage
	}
}

func parseDate(raw, flagName string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s 不得为空", flagName)
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s 需为 RFC3339 或 YYYY-MM-DD, 收到 %q", flagName, raw)
	}
	return t, nil
}

func (a *app) runClaim(args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, errors.New("claim 需要子命令: list / show / validate / advance / finalize / assess")
	}
	fs := flag.NewFlagSet("claim "+args[0], flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	idFlag := fs.String("id", "", "案件编号")
	stageFlag := fs.String("stage", "", "按阶段筛选")
	kindFlag := fs.String("kind", "", "按损害类型筛选")
	toFlag := fs.String("to", "", "目标阶段")
	operatorFlag := fs.String("operator", "赔偿权利人指定部门", "操作人")
	noteFlag := fs.String("note", "", "备注")
	atFlag := fs.String("at", "2026-08-16", "操作日期")
	timeoutFlag := fs.Duration("timeout", 5*time.Second, "外部鉴定评估调用超时")
	assessFirstFlag := fs.Bool("assess-first", false, "校验前先按本地口径核定赔偿金额")
	_ = fs.Duration("gateway-latency", 0, "模拟鉴定评估网关响应耗时")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}

	switch args[0] {
	case "list":
		if *stageFlag != "" {
			stage, err := model.ParseStage(*stageFlag)
			if err != nil {
				return classify(err), err
			}
			return ExitOK, a.emit(map[string]any{"claims": a.registry.ClaimsByStage(stage)})
		}
		if *kindFlag != "" {
			kind, err := model.ParseDamageKind(*kindFlag)
			if err != nil {
				return classify(err), err
			}
			return ExitOK, a.emit(map[string]any{"claims": a.registry.ClaimsByKind(kind)})
		}
		return ExitOK, a.emit(map[string]any{"claims": a.registry.Claims()})

	case "show":
		if *idFlag == "" {
			return ExitBadRequest, errors.New("claim show 需要 --id")
		}
		c, err := a.registry.Claim(*idFlag)
		if err != nil {
			return classify(err), err
		}
		return ExitOK, a.emit(map[string]any{
			"claim":         c,
			"stage_name":    c.Stage.DisplayName(),
			"kind_name":     c.Kind.DisplayName(),
			"allowed_next":  claim.Allowed(c.Stage),
			"missing_items": claim.MissingCount(c),
		})

	case "validate":
		if *idFlag == "" {
			return ExitBadRequest, errors.New("claim validate 需要 --id")
		}
		c, err := a.registry.Claim(*idFlag)
		if err != nil {
			return classify(err), err
		}
		if *assessFirstFlag {
			// 先按本地口径核定金额并写回，使材料齐备，再执行校验。
			b, cerr := assess.Compute(c)
			if cerr != nil {
				return classify(cerr), cerr
			}
			c.AwardedAmount = b.Total
			if serr := a.registry.Save(c); serr != nil {
				return classify(serr), serr
			}
			c, err = a.registry.Claim(*idFlag)
			if err != nil {
				return classify(err), err
			}
		}
		verr := claim.Validate(c)
		payload := map[string]any{
			"claim_id":      c.ID,
			"ok":            verr == nil,
			"missing_items": claim.MissingCount(c),
		}
		if verr != nil {
			payload["message"] = verr.Error()
		}
		if err := a.emit(payload); err != nil {
			return ExitUsage, err
		}
		if verr != nil {
			return classify(verr), verr
		}
		return ExitOK, nil

	case "advance":
		if *idFlag == "" || *toFlag == "" {
			return ExitBadRequest, errors.New("claim advance 需要 --id 和 --to")
		}
		to, err := model.ParseStage(*toFlag)
		if err != nil {
			return classify(err), err
		}
		at, perr := parseDate(*atFlag, "--at")
		if perr != nil {
			return ExitBadRequest, perr
		}
		next, err := a.claims.Advance(*idFlag, to, *operatorFlag, *noteFlag, at)
		if err != nil {
			return classify(err), err
		}
		return ExitOK, a.emit(map[string]any{
			"claim_id":   next.ID,
			"stage":      string(next.Stage),
			"stage_name": next.Stage.DisplayName(),
			"history":    len(next.Transitions),
		})

	case "finalize":
		if *idFlag == "" {
			return ExitBadRequest, errors.New("claim finalize 需要 --id")
		}
		at, perr := parseDate(*atFlag, "--at")
		if perr != nil {
			return ExitBadRequest, perr
		}
		before, err := a.registry.Claim(*idFlag)
		if err != nil {
			return classify(err), err
		}
		finalizeErr := a.claims.Finalize(*idFlag, *operatorFlag, at)
		after, err := a.registry.Claim(*idFlag)
		if err != nil {
			return classify(err), err
		}
		payload := map[string]any{
			"claim_id":     *idFlag,
			"stage_before": string(before.Stage),
			"stage_after":  string(after.Stage),
			"finalized":    after.Stage == model.StageClosed,
			"ok":           finalizeErr == nil,
		}
		if finalizeErr != nil {
			payload["message"] = finalizeErr.Error()
		}
		if err := a.emit(payload); err != nil {
			return ExitUsage, err
		}
		if finalizeErr != nil {
			return classify(finalizeErr), finalizeErr
		}
		return ExitOK, nil

	case "assess":
		if *idFlag == "" {
			return ExitBadRequest, errors.New("claim assess 需要 --id")
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
		defer cancel()

		begin := time.Now()
		breakdown, aerr := a.claims.Assess(ctx, *idFlag)
		elapsed := time.Since(begin)

		payload := map[string]any{
			"claim_id":   *idFlag,
			"timeout_ms": timeoutFlag.Milliseconds(),
			"elapsed_ms": elapsed.Milliseconds(),
			"ok":         aerr == nil,
		}
		if aerr != nil {
			payload["message"] = aerr.Error()
		} else {
			payload["breakdown"] = breakdown
			payload["describe"] = breakdown.Describe()
		}
		if err := a.emit(payload); err != nil {
			return ExitUsage, err
		}
		if aerr != nil {
			return classify(aerr), aerr
		}
		return ExitOK, nil

	default:
		return ExitUsage, fmt.Errorf("未知子命令 claim %q", args[0])
	}
}

func (a *app) runDeadline(args []string) (int, error) {
	if len(args) == 0 || args[0] != "show" {
		return ExitUsage, errors.New("deadline 需要子命令: show")
	}
	fs := flag.NewFlagSet("deadline show", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	idFlag := fs.String("id", "", "案件编号")
	atFlag := fs.String("at", "2026-08-16", "参照日期")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}
	if *idFlag == "" {
		return ExitBadRequest, errors.New("deadline show 需要 --id")
	}
	at, perr := parseDate(*atFlag, "--at")
	if perr != nil {
		return ExitBadRequest, perr
	}
	schedule, err := a.claims.Schedule(*idFlag)
	if err != nil {
		return classify(err), err
	}
	return ExitOK, a.emit(map[string]any{
		"claim_id":             schedule.ClaimID,
		"filed_at":             schedule.FiledAt.Format("2006-01-02"),
		"filed_weekday":        schedule.FiledAt.Weekday().String(),
		"notice_workdays":      deadline.NoticeWorkdays,
		"notice_due":           schedule.NoticeDue.Format("2006-01-02"),
		"notice_due_weekday":   schedule.NoticeDue.Weekday().String(),
		"assessment_due":       schedule.AssessmentDue.Format("2006-01-02"),
		"negotiation_due":      schedule.NegotiationDue.Format("2006-01-02"),
		"litigation_barred_at": schedule.LitigationBarredAt.Format("2006-01-02"),
		"notice_overdue":       schedule.NoticeOverdue(at),
		"remaining_workdays":   schedule.RemainingWorkdaysForNotice(at),
	})
}

func (a *app) runDocket(args []string) (int, error) {
	if len(args) == 0 || args[0] != "settle" {
		return ExitUsage, errors.New("docket 需要子命令: settle")
	}
	fs := flag.NewFlagSet("docket settle", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	workersFlag := fs.Int("workers", 4, "并发度")
	settleFlag := fs.Duration("settle", 20*time.Millisecond, "单件案件核算耗时")
	timeoutFlag := fs.Duration("timeout", 30*time.Second, "整批核算超时")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}
	if *workersFlag <= 0 {
		return ExitBadRequest, errors.New("--workers 必须为正")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	runner := docket.New(*workersFlag, *settleFlag)
	claims := a.registry.Claims()
	res, err := runner.Run(ctx, docket.ItemsFrom(claims))
	if err != nil {
		return classify(err), err
	}

	payload := map[string]any{
		"requested":  res.Requested,
		"completed":  res.Completed,
		"failed":     res.Failed,
		"outcomes":   len(res.Outcomes),
		"complete":   res.Complete(),
		"workers":    res.Workers,
		"summary":    res.Summary,
		"elapsed_ms": res.Elapsed.Milliseconds(),
	}
	if err := a.emit(payload); err != nil {
		return ExitUsage, err
	}
	if !res.Complete() {
		return ExitUsage, fmt.Errorf("批量核算结果不完整: 请求 %d 件, 产出 %d 件",
			res.Requested, len(res.Outcomes))
	}
	return ExitOK, nil
}

func (a *app) runReport(args []string) (int, error) {
	if len(args) == 0 {
		return ExitUsage, errors.New("report 需要子命令: docket / provinces / kinds")
	}
	fs := flag.NewFlagSet("report "+args[0], flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	atFlag := fs.String("at", "2026-08-16", "报表日期")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}
	at, perr := parseDate(*atFlag, "--at")
	if perr != nil {
		return ExitBadRequest, perr
	}
	switch args[0] {
	case "docket":
		rep, err := a.reports.Docket(at)
		if err != nil {
			return classify(err), err
		}
		return ExitOK, a.emit(rep)
	case "provinces":
		lines, err := a.reports.Provinces()
		if err != nil {
			return classify(err), err
		}
		return ExitOK, a.emit(map[string]any{"provinces": lines})
	case "kinds":
		lines, err := a.reports.Kinds()
		if err != nil {
			return classify(err), err
		}
		return ExitOK, a.emit(map[string]any{"kinds": lines})
	default:
		return ExitUsage, fmt.Errorf("未知子命令 report %q", args[0])
	}
}

func (a *app) runGateway(args []string) (int, error) {
	if len(args) == 0 || args[0] != "probe" {
		return ExitUsage, errors.New("gateway 需要子命令: probe")
	}
	fs := flag.NewFlagSet("gateway probe", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	timeoutFlag := fs.Duration("timeout", 2*time.Second, "探测超时")
	_ = fs.Duration("gateway-latency", 0, "模拟鉴定评估网关响应耗时")
	if err := fs.Parse(args[1:]); err != nil {
		return ExitUsage, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeoutFlag)
	defer cancel()

	begin := time.Now()
	perr := a.claims.Probe(ctx)
	elapsed := time.Since(begin)

	payload := map[string]any{
		"institute":  a.gateway.Institute(),
		"timeout_ms": timeoutFlag.Milliseconds(),
		"elapsed_ms": elapsed.Milliseconds(),
		"ok":         perr == nil,
	}
	if perr != nil {
		payload["message"] = perr.Error()
	}
	if err := a.emit(payload); err != nil {
		return ExitUsage, err
	}
	if perr != nil {
		return classify(perr), perr
	}
	return ExitOK, nil
}

func (a *app) runServe(args []string) (int, error) {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	addrFlag := fs.String("addr", "127.0.0.1:8080", "监听地址")
	_ = fs.Duration("gateway-latency", 0, "模拟鉴定评估网关响应耗时")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}
	srv := httpapi.New(httpapi.Options{Registry: a.registry, Claims: a.claims})
	fmt.Fprintf(a.stdout, "ecoctl serve 监听 %s\n", *addrFlag)
	server := &http.Server{
		Addr:              *addrFlag,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return ExitUsage, err
	}
	return ExitOK, nil
}

func (a *app) runSelfcheck(args []string) (int, error) {
	fs := flag.NewFlagSet("selfcheck", flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	_ = fs.Duration("gateway-latency", 0, "模拟鉴定评估网关响应耗时")
	if err := fs.Parse(args); err != nil {
		return ExitUsage, err
	}

	checks := make([]map[string]any, 0, 8)
	add := func(name string, ok bool, detail string) {
		checks = append(checks, map[string]any{"check": name, "ok": ok, "detail": detail})
	}

	counts := a.registry.Counts()
	add("registry", counts.Claims == len(seed.Claims()) && counts.Respondents == len(seed.Respondents()),
		fmt.Sprintf("案件 %d 件, 赔偿义务人 %d 个", counts.Claims, counts.Respondents))

	// 工作日时限：2026-08-17 周一立案，20 个工作日后应为 2026-09-14 周一。
	probeSchedule, err := deadline.Compute("PROBE", time.Date(2026, time.August, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return classify(err), err
	}
	wantNotice := time.Date(2026, time.September, 14, 0, 0, 0, 0, time.UTC)
	add("notice-workdays", probeSchedule.NoticeDue.Equal(wantNotice),
		fmt.Sprintf("2026-08-17 立案的告知期限 = %s, 期望 %s",
			probeSchedule.NoticeDue.Format("2006-01-02"), wantNotice.Format("2006-01-02")))

	// 材料齐备的案件校验必须返回 nil。
	complete := completeProbeClaim()
	verr := claim.Validate(complete)
	add("validate-complete-claim", verr == nil,
		fmt.Sprintf("材料齐备案件校验结果 = %v", verr))

	// 材料缺失的案件校验必须返回失败。
	incomplete := complete
	incomplete.AwardedAmount = 0
	incomplete.BaselineCost = 0
	ierr := claim.Validate(incomplete)
	add("validate-incomplete-claim", ierr != nil && errors.Is(ierr, model.ErrValidationFailed),
		fmt.Sprintf("材料缺失案件校验结果 = %v", ierr != nil))

	// 前置校验不通过的案件结案必须失败且阶段不变。
	probeReg, perr := seed.Load()
	if perr != nil {
		return classify(perr), perr
	}
	probeSvc := claim.NewService(probeReg, nil)
	target := seed.RestorationClaimIDs()[0]
	beforeStage := ""
	if c, cerr := probeReg.Claim(target); cerr == nil {
		beforeStage = string(c.Stage)
	}
	finalizeErr := probeSvc.Finalize(target, "自检", time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC))
	afterStage := ""
	if c, cerr := probeReg.Claim(target); cerr == nil {
		afterStage = string(c.Stage)
	}
	add("finalize-reports-precheck-failure", finalizeErr != nil && beforeStage == afterStage,
		fmt.Sprintf("未核定金额案件结案返回错误 = %v, 阶段 %s -> %s", finalizeErr != nil, beforeStage, afterStage))

	// 批量核算必须覆盖全部案件。
	runner := docket.New(4, 3*time.Millisecond)
	res, rerr := runner.Run(context.Background(), docket.ItemsFrom(a.registry.Claims()))
	if rerr != nil {
		return classify(rerr), rerr
	}
	add("docket-complete", res.Complete() && len(res.Outcomes) == len(a.registry.Claims()),
		fmt.Sprintf("请求 %d 件, 产出 %d 件", res.Requested, len(res.Outcomes)))

	// 金额核算必须为正且随程度系数放大。
	minor := completeProbeClaim()
	minor.Severity = model.SeverityMinor
	major := completeProbeClaim()
	major.Severity = model.SeverityMajor
	bMinor, e1 := assess.Compute(minor)
	bMajor, e2 := assess.Compute(major)
	add("assess-multiplier", e1 == nil && e2 == nil && bMajor.Total > bMinor.Total,
		fmt.Sprintf("轻微 %.2f 元 < 特别严重 %.2f 元", bMinor.Total, bMajor.Total))

	// 网关必须尊重超时。
	fast := gateway.New(gateway.Options{Latency: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	begin := time.Now()
	gerr := fast.Probe(ctx)
	add("gateway-honours-timeout", gerr != nil && time.Since(begin) < time.Second,
		fmt.Sprintf("30ms 超时下探测返回错误 = %v, 耗时 %v", gerr != nil, time.Since(begin).Round(time.Millisecond)))

	sort.Slice(checks, func(i, j int) bool {
		return checks[i]["check"].(string) < checks[j]["check"].(string)
	})
	failed := 0
	for _, c := range checks {
		if !c["ok"].(bool) {
			failed++
		}
	}
	if err := a.emit(map[string]any{"checks": checks, "failed": failed}); err != nil {
		return ExitUsage, err
	}
	if failed > 0 {
		return ExitUsage, fmt.Errorf("自检失败 %d 项", failed)
	}
	return ExitOK, nil
}

// completeProbeClaim 返回一件材料齐备的自检案件。
func completeProbeClaim() model.Claim {
	filed := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	return model.Claim{
		ID:            "PROBE-1",
		Title:         "自检案件",
		Kind:          model.KindSoil,
		Severity:      model.SeveritySerious,
		Stage:         model.StageRestoration,
		RespondentID:  "R-001",
		Province:      "江苏",
		FiledAt:       filed,
		AffectedArea:  50,
		AffectedDays:  100,
		BaselineCost:  200000,
		Restorable:    true,
		AwardedAmount: 1,
		Transitions: []model.Transition{
			{From: model.StageIntake, To: model.StageInvestigation, At: filed.AddDate(0, 0, 7), Operator: "自检"},
		},
	}
}
