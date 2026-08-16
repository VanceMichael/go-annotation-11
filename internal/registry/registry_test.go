package registry

import (
	"errors"
	"testing"
	"time"

	"ecoclaim/internal/model"
)

func sampleClaim(id string) model.Claim {
	filed := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	return model.Claim{
		ID:           id,
		Title:        "测试案件",
		Kind:         model.KindSoil,
		Severity:     model.SeveritySerious,
		Stage:        model.StageIntake,
		RespondentID: "R-001",
		Province:     "江苏",
		FiledAt:      filed,
		AffectedArea: 50,
		AffectedDays: 100,
		BaselineCost: 100000,
		Restorable:   true,
	}
}

func seeded(t *testing.T) *Registry {
	t.Helper()
	r := New()
	if err := r.AddRespondent(model.Respondent{ID: "R-001", Name: "某公司", Kind: "企业", Province: "江苏"}); err != nil {
		t.Fatalf("AddRespondent 失败: %v", err)
	}
	return r
}

func TestAddRequiresRespondent(t *testing.T) {
	r := New()
	if err := r.Add(sampleClaim("HJ-1")); !errors.Is(err, model.ErrRespondentUnknown) {
		t.Fatalf("赔偿义务人未登记应返回 ErrRespondentUnknown, 实际 %v", err)
	}
}

func TestAddDuplicate(t *testing.T) {
	r := seeded(t)
	if err := r.Add(sampleClaim("HJ-1")); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	if err := r.Add(sampleClaim("HJ-1")); !errors.Is(err, model.ErrDuplicateClaim) {
		t.Fatalf("重复登记应返回 ErrDuplicateClaim, 实际 %v", err)
	}
}

func TestAddRejectsInvalid(t *testing.T) {
	r := seeded(t)
	c := sampleClaim("")
	if err := r.Add(c); !errors.Is(err, model.ErrInvalidClaim) {
		t.Fatalf("非法案件应返回 ErrInvalidClaim, 实际 %v", err)
	}
}

func TestClaimAndSave(t *testing.T) {
	r := seeded(t)
	c := sampleClaim("HJ-1")
	if err := r.Add(c); err != nil {
		t.Fatalf("Add 失败: %v", err)
	}
	got, err := r.Claim("HJ-1")
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if got.Stage != model.StageIntake {
		t.Fatalf("阶段 = %s", got.Stage)
	}

	got.Stage = model.StageInvestigation
	if err := r.Save(got); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	again, err := r.Claim("HJ-1")
	if err != nil {
		t.Fatalf("Claim 失败: %v", err)
	}
	if again.Stage != model.StageInvestigation {
		t.Fatalf("持久化阶段 = %s", again.Stage)
	}

	if _, err := r.Claim("NOPE"); !errors.Is(err, model.ErrClaimUnknown) {
		t.Fatalf("未知案件应返回 ErrClaimUnknown, 实际 %v", err)
	}
	if err := r.Save(sampleClaim("NOPE")); !errors.Is(err, model.ErrClaimUnknown) {
		t.Fatalf("保存未登记案件应返回 ErrClaimUnknown, 实际 %v", err)
	}
}

func TestFiltersAndCounts(t *testing.T) {
	r := seeded(t)
	for i, spec := range []struct {
		id    string
		kind  model.DamageKind
		stage model.Stage
	}{
		{"HJ-1", model.KindSoil, model.StageIntake},
		{"HJ-2", model.KindWater, model.StageRestoration},
		{"HJ-3", model.KindSoil, model.StageClosed},
	} {
		c := sampleClaim(spec.id)
		c.Kind = spec.kind
		c.Stage = spec.stage
		c.FiledAt = c.FiledAt.AddDate(0, 0, i)
		if err := r.Add(c); err != nil {
			t.Fatalf("Add %s 失败: %v", spec.id, err)
		}
	}

	if got := r.ClaimsByKind(model.KindSoil); len(got) != 2 {
		t.Fatalf("土壤案件数 = %d, 期望 2", len(got))
	}
	if got := r.ClaimsByStage(model.StageClosed); len(got) != 1 {
		t.Fatalf("结案案件数 = %d, 期望 1", len(got))
	}

	c := r.Counts()
	if c.Claims != 3 || c.Closed != 1 || c.Open != 2 || c.Respondents != 1 {
		t.Fatalf("Counts = %+v", c)
	}
	if got := r.StageCounts()["intake"]; got != 1 {
		t.Fatalf("intake 数 = %d", got)
	}
	if got := r.IDs(); len(got) != 3 || got[0] != "HJ-1" {
		t.Fatalf("IDs = %v", got)
	}
	if got := r.Claims(); len(got) != 3 || got[0].ID != "HJ-1" {
		t.Fatalf("Claims 排序异常: %+v", got)
	}
}

func TestRespondentLookup(t *testing.T) {
	r := seeded(t)
	if _, err := r.Respondent("R-001"); err != nil {
		t.Fatalf("Respondent 失败: %v", err)
	}
	if _, err := r.Respondent("NOPE"); !errors.Is(err, model.ErrRespondentUnknown) {
		t.Fatalf("未知义务人应返回 ErrRespondentUnknown, 实际 %v", err)
	}
	if got := len(r.Respondents()); got != 1 {
		t.Fatalf("义务人数 = %d", got)
	}
}

func TestAddRespondentValidation(t *testing.T) {
	r := New()
	if err := r.AddRespondent(model.Respondent{Name: "x"}); err == nil {
		t.Errorf("缺少编号应返回错误")
	}
	if err := r.AddRespondent(model.Respondent{ID: "R-1"}); err == nil {
		t.Errorf("缺少名称应返回错误")
	}
}
