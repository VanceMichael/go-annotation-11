package claim

import (
	"context"
	"errors"
	"testing"
	"time"

	"ecoclaim/internal/gateway"
	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
	"ecoclaim/internal/seed"
)

func at(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func fixture(t *testing.T, latency time.Duration) (*Service, *registry.Registry) {
	t.Helper()
	reg, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load 失败: %v", err)
	}
	return NewService(reg, gateway.New(gateway.Options{Latency: latency})), reg
}

func restorationClaimID(t *testing.T) string {
	t.Helper()
	ids := seed.RestorationClaimIDs()
	if len(ids) == 0 {
		t.Fatalf("内置数据缺少处于修复实施阶段的案件")
	}
	return ids[0]
}

// TestValidateReturnsNilForCompleteClaim 断言材料齐备的案件校验返回 nil。
func TestValidateReturnsNilForCompleteClaim(t *testing.T) {
	c := completeClaim()
	if err := Validate(c); err != nil {
		t.Fatalf("材料齐备的案件校验应返回 nil, 实际 %v (类型 %T)", err, err)
	}
}

// TestValidateNilCheckIsUsableByCaller 断言调用方用 err != nil 判定校验结果时，
// 材料齐备的案件不会被误判为失败。
func TestValidateNilCheckIsUsableByCaller(t *testing.T) {
	c := completeClaim()
	err := Validate(c)
	if err != nil {
		t.Fatalf("err != nil 为真, 材料齐备的案件被误判为校验失败; 错误值 %v (类型 %T)", err, err)
	}
	if MissingCount(c) != 0 {
		t.Fatalf("缺失项数 = %d, 期望 0", MissingCount(c))
	}
}

// TestValidateAcrossCompleteVariants 断言多种材料齐备形态均返回 nil。
func TestValidateAcrossCompleteVariants(t *testing.T) {
	variants := []struct {
		name   string
		mutate func(*model.Claim)
	}{
		{"土壤污染可修复", func(c *model.Claim) {}},
		{"噪声案件无面积", func(c *model.Claim) { c.Kind = model.KindNoise; c.AffectedArea = 0 }},
		{"不可原地修复", func(c *model.Claim) { c.Restorable = false }},
		{"特别严重", func(c *model.Claim) { c.Severity = model.SeverityMajor }},
	}
	for _, v := range variants {
		c := completeClaim()
		v.mutate(&c)
		if err := Validate(c); err != nil {
			t.Errorf("%s: 校验应返回 nil, 实际 %v (类型 %T)", v.name, err, err)
		}
	}
}

// TestValidateReportsMissingItems 断言材料缺失时返回可识别的校验失败错误。
func TestValidateReportsMissingItems(t *testing.T) {
	c := completeClaim()
	c.AwardedAmount = 0
	c.BaselineCost = 0

	err := Validate(c)
	if err == nil {
		t.Fatalf("材料缺失应返回错误")
	}
	if !errors.Is(err, model.ErrValidationFailed) {
		t.Fatalf("errors.Is(err, model.ErrValidationFailed) = false, 错误为 %v", err)
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As 未能取出 *ValidationError, 错误为 %v", err)
	}
	if len(verr.Items()) != 2 {
		t.Fatalf("缺失项 = %v, 期望 2 项", verr.Items())
	}
	if verr.Error() == "" {
		t.Fatalf("错误描述为空")
	}
}

func TestMissingCount(t *testing.T) {
	c := completeClaim()
	if got := MissingCount(c); got != 0 {
		t.Fatalf("齐备案件缺失项 = %d, 期望 0", got)
	}
	c.Transitions = nil
	c.AwardedAmount = 0
	if got := MissingCount(c); got != 2 {
		t.Fatalf("缺失项 = %d, 期望 2", got)
	}
}

// TestFinalizeReportsPrecheckFailure 断言前置校验不通过时结案返回错误且阶段不变。
func TestFinalizeReportsPrecheckFailure(t *testing.T) {
	svc, reg := fixture(t, 0)
	id := restorationClaimID(t)

	before, err := reg.Claim(id)
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if before.AwardedAmount > 0 {
		t.Fatalf("前置条件不成立: 案件 %s 已核定金额", id)
	}

	ferr := svc.Finalize(id, "测试", at(2026, time.August, 16))
	if ferr == nil {
		t.Fatalf("未核定赔偿金额的案件结案应返回错误, 实际返回 nil")
	}

	after, err := reg.Claim(id)
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if after.Stage != before.Stage {
		t.Fatalf("结案失败时阶段不应变化: %s -> %s", before.Stage, after.Stage)
	}
	if after.Stage == model.StageClosed {
		t.Fatalf("前置校验未通过的案件不应被标记为结案")
	}
}

// TestFinalizeSurfacesValidationSentinel 断言材料缺失导致的结案失败可被识别。
func TestFinalizeSurfacesValidationSentinel(t *testing.T) {
	reg := registry.New()
	if err := reg.AddRespondent(model.Respondent{ID: "R-001", Name: "某公司"}); err != nil {
		t.Fatalf("AddRespondent 失败: %v", err)
	}
	c := completeClaim()
	c.Transitions = nil // 缺少阶段流转记录
	c.Stage = model.StageRestoration
	if err := reg.Add(c); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}

	svc := NewService(reg, nil)
	err := svc.Finalize(c.ID, "测试", at(2026, time.August, 16))
	if err == nil {
		t.Fatalf("材料缺失的案件结案应返回错误")
	}
	if !errors.Is(err, model.ErrValidationFailed) {
		t.Fatalf("errors.Is(err, model.ErrValidationFailed) = false, 错误为 %v", err)
	}
	after, _ := reg.Claim(c.ID)
	if after.Stage == model.StageClosed {
		t.Fatalf("案件不应被结案")
	}
}

// TestFinalizeSucceedsWhenReady 断言材料齐备且已核定金额的案件可以结案。
func TestFinalizeSucceedsWhenReady(t *testing.T) {
	reg := registry.New()
	if err := reg.AddRespondent(model.Respondent{ID: "R-001", Name: "某公司"}); err != nil {
		t.Fatalf("AddRespondent 失败: %v", err)
	}
	c := completeClaim()
	c.Stage = model.StageRestoration
	c.AwardedAmount = 1250000
	if err := reg.Add(c); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}

	svc := NewService(reg, nil)
	if err := svc.Finalize(c.ID, "测试", at(2026, time.August, 16)); err != nil {
		t.Fatalf("齐备案件结案应成功, 实际 %v", err)
	}
	after, err := reg.Claim(c.ID)
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if after.Stage != model.StageClosed {
		t.Fatalf("结案后阶段 = %s, 期望 %s", after.Stage, model.StageClosed)
	}
	if len(after.Transitions) != len(c.Transitions)+1 {
		t.Fatalf("结案后流转记录 = %d 条, 期望 %d 条", len(after.Transitions), len(c.Transitions)+1)
	}
}

func TestFinalizeUnknownClaim(t *testing.T) {
	svc, _ := fixture(t, 0)
	if err := svc.Finalize("NOPE", "测试", at(2026, time.August, 16)); !errors.Is(err, model.ErrClaimUnknown) {
		t.Fatalf("未知案件应返回 ErrClaimUnknown, 实际 %v", err)
	}
}

func TestAdvanceStageConflict(t *testing.T) {
	svc, _ := fixture(t, 0)
	// HJ-2026-007 处于线索登记阶段，不能直接流转到结案。
	if _, err := svc.Advance("HJ-2026-007", model.StageClosed, "测试", "", at(2026, time.August, 16)); !errors.Is(err, model.ErrStageConflict) {
		t.Fatalf("非法流转应返回 ErrStageConflict, 实际 %v", err)
	}
}

func TestAdvanceSuccess(t *testing.T) {
	svc, reg := fixture(t, 0)
	next, err := svc.Advance("HJ-2026-007", model.StageInvestigation, "测试", "开展调查", at(2026, time.August, 16))
	if err != nil {
		t.Fatalf("Advance 失败: %v", err)
	}
	if next.Stage != model.StageInvestigation {
		t.Fatalf("阶段 = %s", next.Stage)
	}
	saved, err := reg.Claim("HJ-2026-007")
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if saved.Stage != model.StageInvestigation {
		t.Fatalf("持久化阶段 = %s", saved.Stage)
	}
}

func TestAdvanceDeadlineExpired(t *testing.T) {
	svc, _ := fixture(t, 0)
	// 立案 3 年后诉讼时效届满。
	if _, err := svc.Advance("HJ-2026-007", model.StageInvestigation, "测试", "", at(2030, time.January, 1)); !errors.Is(err, model.ErrDeadlineExpired) {
		t.Fatalf("时效届满应返回 ErrDeadlineExpired, 实际 %v", err)
	}
}

func TestAllowedStages(t *testing.T) {
	if got := Allowed(model.StageIntake); len(got) != 2 {
		t.Fatalf("线索登记允许流转 = %v", got)
	}
	if got := Allowed(model.StageClosed); len(got) != 0 {
		t.Fatalf("结案不应允许再流转, 实际 %v", got)
	}
}

// TestAssessHonoursContextTimeout 断言核定金额时调用方超时会立即返回。
func TestAssessHonoursContextTimeout(t *testing.T) {
	svc, _ := fixture(t, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	begin := time.Now()
	_, err := svc.Assess(ctx, "HJ-2026-001")
	elapsed := time.Since(begin)

	if err == nil {
		t.Fatalf("超时后 Assess 应返回错误")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false, 错误为 %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("超时后耗时 = %v, 期望远小于 1s（网关耗时 5s）", elapsed)
	}
}

// TestAssessHonoursContextCancel 断言核定金额时调用方取消会立即返回。
func TestAssessHonoursContextCancel(t *testing.T) {
	svc, _ := fixture(t, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(60 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	begin := time.Now()
	_, err := svc.Assess(ctx, "HJ-2026-001")
	elapsed := time.Since(begin)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false, 错误为 %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("取消后耗时 = %v, 期望远小于 1s", elapsed)
	}
}

// TestProbeHonoursContextTimeout 断言网关探测尊重调用方超时。
func TestProbeHonoursContextTimeout(t *testing.T) {
	svc, _ := fixture(t, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()

	begin := time.Now()
	err := svc.Probe(ctx)
	elapsed := time.Since(begin)

	if err == nil {
		t.Fatalf("超时后 Probe 应返回错误")
	}
	if elapsed > time.Second {
		t.Fatalf("超时后耗时 = %v, 期望远小于 1s", elapsed)
	}
}

func TestAssessSucceedsWithFastGateway(t *testing.T) {
	svc, reg := fixture(t, time.Millisecond)
	breakdown, err := svc.Assess(context.Background(), "HJ-2026-001")
	if err != nil {
		t.Fatalf("Assess 失败: %v", err)
	}
	if breakdown.Total <= 0 {
		t.Fatalf("核定金额 = %.2f, 应为正", breakdown.Total)
	}
	saved, err := reg.Claim("HJ-2026-001")
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if saved.AwardedAmount != breakdown.Total {
		t.Fatalf("持久化金额 = %.2f, 期望 %.2f", saved.AwardedAmount, breakdown.Total)
	}
}

func TestAssessRejectsIntakeStage(t *testing.T) {
	svc, _ := fixture(t, 0)
	if _, err := svc.Assess(context.Background(), "HJ-2026-007"); !errors.Is(err, model.ErrAssessmentIncomplete) {
		t.Fatalf("线索登记阶段应返回 ErrAssessmentIncomplete, 实际 %v", err)
	}
}

func TestSchedule(t *testing.T) {
	svc, _ := fixture(t, 0)
	schedule, err := svc.Schedule("HJ-2026-001")
	if err != nil {
		t.Fatalf("Schedule 失败: %v", err)
	}
	if schedule.ClaimID != "HJ-2026-001" {
		t.Fatalf("ClaimID = %s", schedule.ClaimID)
	}
	if schedule.NoticeDue.Before(schedule.FiledAt) {
		t.Fatalf("告知期限早于立案日期")
	}
}

// completeClaim 返回一件材料齐备的案件。
func completeClaim() model.Claim {
	filed := at(2026, time.March, 9)
	return model.Claim{
		ID:            "HJ-TEST-001",
		Title:         "材料齐备的测试案件",
		Kind:          model.KindSoil,
		Severity:      model.SeveritySerious,
		Stage:         model.StageRestoration,
		RespondentID:  "R-001",
		Province:      "江苏",
		FiledAt:       filed,
		AffectedArea:  86.5,
		AffectedDays:  142,
		BaselineCost:  380000,
		Restorable:    true,
		AwardedAmount: 1200000,
		Transitions: []model.Transition{
			{From: model.StageIntake, To: model.StageInvestigation, At: filed.AddDate(0, 0, 7), Operator: "测试"},
		},
	}
}
