package model

import (
	"errors"
	"testing"
	"time"
)

func validClaim() Claim {
	filed := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	return Claim{
		ID: "HJ-1", Title: "测试案件", Kind: KindSoil, Severity: SeveritySerious,
		Stage: StageIntake, RespondentID: "R-001", Province: "江苏", FiledAt: filed,
		AffectedArea: 50, AffectedDays: 100, BaselineCost: 100000, Restorable: true,
	}
}

func TestParseDamageKind(t *testing.T) {
	for _, k := range AllDamageKinds() {
		got, err := ParseDamageKind(string(k))
		if err != nil || got != k {
			t.Fatalf("ParseDamageKind(%q) = %q, %v", k, got, err)
		}
		if k.DisplayName() == "" {
			t.Errorf("%s 缺少中文名", k)
		}
	}
	if _, err := ParseDamageKind("  SOIL "); err != nil {
		t.Fatalf("应忽略大小写与空白: %v", err)
	}
	if _, err := ParseDamageKind("radiation"); !errors.Is(err, ErrUnknownDamageKind) {
		t.Fatalf("未知类型应返回 ErrUnknownDamageKind, 实际 %v", err)
	}
	if DamageKind("x").DisplayName() != "x" {
		t.Errorf("未知类型应回落为原值")
	}
}

func TestRestorationFeasible(t *testing.T) {
	feasible := map[DamageKind]bool{KindSoil: true, KindWater: true, KindEcology: true}
	for _, k := range AllDamageKinds() {
		if got := k.RestorationFeasible(); got != feasible[k] {
			t.Errorf("%s.RestorationFeasible() = %v, 期望 %v", k, got, feasible[k])
		}
	}
}

func TestParseStage(t *testing.T) {
	for _, s := range AllStages() {
		got, err := ParseStage(string(s))
		if err != nil || got != s {
			t.Fatalf("ParseStage(%q) = %q, %v", s, got, err)
		}
		if s.DisplayName() == "" {
			t.Errorf("%s 缺少中文名", s)
		}
	}
	if _, err := ParseStage("appeal"); !errors.Is(err, ErrUnknownStage) {
		t.Fatalf("未知阶段应返回 ErrUnknownStage, 实际 %v", err)
	}
	if Stage("x").DisplayName() != "x" {
		t.Errorf("未知阶段应回落为原值")
	}
}

func TestStageTerminal(t *testing.T) {
	terminal := map[Stage]bool{StageClosed: true, StageTerminated: true}
	for _, s := range AllStages() {
		if got := s.Terminal(); got != terminal[s] {
			t.Errorf("%s.Terminal() = %v, 期望 %v", s, got, terminal[s])
		}
	}
}

func TestParseSeverityAndMultiplier(t *testing.T) {
	order := []Severity{SeverityMinor, SeverityModerate, SeveritySerious, SeverityMajor}
	prev := 0.0
	for _, s := range order {
		got, err := ParseSeverity(string(s))
		if err != nil || got != s {
			t.Fatalf("ParseSeverity(%q) = %q, %v", s, got, err)
		}
		m := s.Multiplier()
		if m <= prev {
			t.Fatalf("%s 系数 %.2f 应高于上一档 %.2f", s, m, prev)
		}
		prev = m
		if s.DisplayName() == "" {
			t.Errorf("%s 缺少中文名", s)
		}
	}
	if _, err := ParseSeverity("catastrophic"); !errors.Is(err, ErrUnknownSeverity) {
		t.Fatalf("未知程度应返回 ErrUnknownSeverity, 实际 %v", err)
	}
	if Severity("x").Multiplier() != 1.0 {
		t.Errorf("未知程度系数应为 1.0")
	}
	if Severity("x").DisplayName() != "x" {
		t.Errorf("未知程度应回落为原值")
	}
}

func TestClaimValidate(t *testing.T) {
	if err := validClaim().Validate(); err != nil {
		t.Fatalf("合法案件不应报错: %v", err)
	}
	mutations := []func(c *Claim){
		func(c *Claim) { c.ID = "" },
		func(c *Claim) { c.Title = " " },
		func(c *Claim) { c.Kind = "radiation" },
		func(c *Claim) { c.Severity = "catastrophic" },
		func(c *Claim) { c.RespondentID = "" },
		func(c *Claim) { c.FiledAt = time.Time{} },
		func(c *Claim) { c.AffectedArea = -1 },
		func(c *Claim) { c.AffectedDays = -1 },
		func(c *Claim) { c.BaselineCost = -1 },
	}
	for i, mutate := range mutations {
		c := validClaim()
		mutate(&c)
		if err := c.Validate(); !errors.Is(err, ErrInvalidClaim) {
			t.Errorf("第 %d 项非法输入应返回 ErrInvalidClaim, 实际 %v", i, err)
		}
	}
}

func TestLastTransition(t *testing.T) {
	c := validClaim()
	if _, ok := c.LastTransition(); ok {
		t.Errorf("空历史不应返回流转记录")
	}
	c.Transitions = []Transition{
		{From: StageIntake, To: StageInvestigation},
		{From: StageInvestigation, To: StageAssessment},
	}
	last, ok := c.LastTransition()
	if !ok || last.To != StageAssessment {
		t.Fatalf("LastTransition = %+v, %v", last, ok)
	}
}

func TestSortClaims(t *testing.T) {
	t0 := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	items := []Claim{
		{ID: "B", FiledAt: t0.AddDate(0, 0, 1)},
		{ID: "Z", FiledAt: t0},
		{ID: "A", FiledAt: t0},
	}
	SortClaims(items)
	if items[0].ID != "A" || items[1].ID != "Z" || items[2].ID != "B" {
		t.Fatalf("排序结果 = %+v", items)
	}
}
