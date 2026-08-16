package assess

import (
	"errors"
	"math"
	"testing"
	"time"

	"ecoclaim/internal/model"
)

func base() model.Claim {
	filed := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	return model.Claim{
		ID:           "HJ-A-001",
		Title:        "核算测试案件",
		Kind:         model.KindSoil,
		Severity:     model.SeverityModerate,
		Stage:        model.StageAssessment,
		RespondentID: "R-001",
		FiledAt:      filed,
		AffectedArea: 100,
		AffectedDays: 200,
		BaselineCost: 100000,
		Restorable:   true,
	}
}

func TestComputeBreakdown(t *testing.T) {
	c := base()
	b, err := Compute(c)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if b.Cleanup != 100*CleanupRatePerMu {
		t.Errorf("清污费用 = %.2f", b.Cleanup)
	}
	if b.Restoration != 100*RestorationRatePerMu {
		t.Errorf("修复费用 = %.2f", b.Restoration)
	}
	if b.Interim != 100*200*InterimRatePerMuDay {
		t.Errorf("期间损害 = %.2f", b.Interim)
	}
	direct := b.Cleanup + b.Restoration + b.Interim + b.Baseline
	wantSurvey := math.Max(SurveyFeeFloor, direct*SurveyFeeRate)
	if math.Abs(b.Survey-wantSurvey) > 0.01 {
		t.Errorf("调查鉴定费 = %.2f, 期望 %.2f", b.Survey, wantSurvey)
	}
	if math.Abs(b.Total-(direct+b.Survey)*1.5) > 0.01 {
		t.Errorf("合计 = %.2f", b.Total)
	}
	if b.Alternative {
		t.Errorf("可修复案件不应按替代修复计价")
	}
}

func TestComputeAlternativeRestoration(t *testing.T) {
	c := base()
	c.Restorable = false
	b, err := Compute(c)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if !b.Alternative {
		t.Fatalf("不可原地修复应按替代修复计价")
	}
	if b.Restoration != 100*AlternativeRatePerMu {
		t.Fatalf("替代修复费用 = %.2f", b.Restoration)
	}
}

// TestSeverityMultiplierOrdering 断言赔偿金额随损害程度递增。
func TestSeverityMultiplierOrdering(t *testing.T) {
	prev := 0.0
	for _, s := range []model.Severity{model.SeverityMinor, model.SeverityModerate, model.SeveritySerious, model.SeverityMajor} {
		c := base()
		c.Severity = s
		b, err := Compute(c)
		if err != nil {
			t.Fatalf("%s: Compute 返回错误: %v", s, err)
		}
		if b.Total <= prev {
			t.Fatalf("%s 合计 %.2f 应高于上一档 %.2f", s, b.Total, prev)
		}
		prev = b.Total
	}
}

func TestSurveyFeeFloor(t *testing.T) {
	c := base()
	c.AffectedArea = 0.1
	c.AffectedDays = 1
	c.BaselineCost = 100
	b, err := Compute(c)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if b.Survey != SurveyFeeFloor {
		t.Fatalf("小额案件调查鉴定费 = %.2f, 期望下限 %.2f", b.Survey, SurveyFeeFloor)
	}
}

func TestComputeRejectsInvalid(t *testing.T) {
	c := base()
	c.Severity = model.Severity("unknown")
	if _, err := Compute(c); !errors.Is(err, model.ErrInvalidClaim) && !errors.Is(err, model.ErrUnknownSeverity) {
		t.Fatalf("非法程度应返回错误, 实际 %v", err)
	}
	c2 := base()
	c2.ID = ""
	if _, err := Compute(c2); !errors.Is(err, model.ErrInvalidClaim) {
		t.Fatalf("缺少编号应返回 ErrInvalidClaim, 实际 %v", err)
	}
}

func TestAggregate(t *testing.T) {
	items := []Breakdown{
		{ClaimID: "A", Total: 100},
		{ClaimID: "B", Total: 300},
		{ClaimID: "C", Total: 200},
	}
	s := Aggregate(items)
	if s.Claims != 3 {
		t.Errorf("件数 = %d", s.Claims)
	}
	if s.TotalYuan != 600 {
		t.Errorf("合计 = %.2f", s.TotalYuan)
	}
	if s.MaxYuan != 300 || s.MaxClaimID != "B" {
		t.Errorf("最大值 = %.2f / %s", s.MaxYuan, s.MaxClaimID)
	}
	if s.MinYuan != 100 {
		t.Errorf("最小值 = %.2f", s.MinYuan)
	}
	if s.MeanYuan != 200 {
		t.Errorf("均值 = %.2f", s.MeanYuan)
	}
}

func TestAggregateEmpty(t *testing.T) {
	s := Aggregate(nil)
	if s.Claims != 0 || s.TotalYuan != 0 {
		t.Fatalf("空输入汇总 = %+v", s)
	}
}

func TestDescribe(t *testing.T) {
	b, err := Compute(base())
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if b.Describe() == "" {
		t.Fatalf("描述为空")
	}
	c := base()
	c.Restorable = false
	b2, err := Compute(c)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if b2.Describe() == b.Describe() {
		t.Fatalf("替代修复描述应与原地修复不同")
	}
}

func TestRoundToCents(t *testing.T) {
	c := base()
	c.AffectedArea = 1.0 / 3.0
	b, err := Compute(c)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if b.Total != math.Round(b.Total*100)/100 {
		t.Fatalf("合计未四舍五入到分: %.6f", b.Total)
	}
}
