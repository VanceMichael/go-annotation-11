package report

import (
	"testing"
	"time"

	"ecoclaim/internal/claim"
	"ecoclaim/internal/model"
	"ecoclaim/internal/seed"
)

func builder(t *testing.T) *Builder {
	t.Helper()
	reg, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load 失败: %v", err)
	}
	return NewBuilder(reg, claim.MissingCount)
}

func at() time.Time {
	return time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)
}

func TestDocketCoversEveryClaim(t *testing.T) {
	b := builder(t)
	rep, err := b.Docket(at())
	if err != nil {
		t.Fatalf("Docket 返回错误: %v", err)
	}
	if len(rep.Lines) != len(seed.Claims()) {
		t.Fatalf("台账行数 = %d, 期望 %d", len(rep.Lines), len(seed.Claims()))
	}
	for i := 1; i < len(rep.Lines); i++ {
		if rep.Lines[i-1].ClaimID > rep.Lines[i].ClaimID {
			t.Fatalf("台账未按案件编号排序")
		}
	}
	if rep.Summary.Claims != len(seed.Claims()) {
		t.Fatalf("汇总件数 = %d, 期望 %d", rep.Summary.Claims, len(seed.Claims()))
	}
}

// TestDocketNoticeDueUsesTwentyWorkdays 断言台账中的告知期限为立案后 20 个工作日。
func TestDocketNoticeDueUsesTwentyWorkdays(t *testing.T) {
	b := builder(t)
	rep, err := b.Docket(at())
	if err != nil {
		t.Fatalf("Docket 返回错误: %v", err)
	}
	byID := map[string]ClaimLine{}
	for _, l := range rep.Lines {
		byID[l.ClaimID] = l
	}
	// HJ-2026-001 立案 2026-03-09（周一），20 个工作日后为 2026-04-06（周一）。
	line, ok := byID["HJ-2026-001"]
	if !ok {
		t.Fatalf("缺少 HJ-2026-001")
	}
	if line.FiledAt != "2026-03-09" {
		t.Fatalf("filed_at = %s", line.FiledAt)
	}
	if line.NoticeDue != "2026-04-06" {
		t.Fatalf("notice_due = %s, 期望 2026-04-06", line.NoticeDue)
	}
}

func TestDocketStageAndKindCounts(t *testing.T) {
	b := builder(t)
	rep, err := b.Docket(at())
	if err != nil {
		t.Fatalf("Docket 返回错误: %v", err)
	}
	var total int
	for _, n := range rep.StageCounts {
		total += n
	}
	if total != len(seed.Claims()) {
		t.Fatalf("阶段计数之和 = %d, 期望 %d", total, len(seed.Claims()))
	}
	total = 0
	for _, n := range rep.KindCounts {
		total += n
	}
	if total != len(seed.Claims()) {
		t.Fatalf("类型计数之和 = %d, 期望 %d", total, len(seed.Claims()))
	}
}

func TestDocketRespondentNamesResolved(t *testing.T) {
	b := builder(t)
	rep, err := b.Docket(at())
	if err != nil {
		t.Fatalf("Docket 返回错误: %v", err)
	}
	for _, l := range rep.Lines {
		if l.Respondent == "" {
			t.Errorf("案件 %s 未解析赔偿义务人名称", l.ClaimID)
		}
	}
}

func TestProvincesSortedByAmount(t *testing.T) {
	b := builder(t)
	lines, err := b.Provinces()
	if err != nil {
		t.Fatalf("Provinces 返回错误: %v", err)
	}
	if len(lines) == 0 {
		t.Fatalf("省份报表为空")
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].AssessedYuan > lines[i-1].AssessedYuan {
			t.Fatalf("省份报表未按金额降序: %+v", lines)
		}
	}
	var claims int
	for _, l := range lines {
		claims += l.Claims
	}
	if claims != len(seed.Claims()) {
		t.Fatalf("省份案件数之和 = %d, 期望 %d", claims, len(seed.Claims()))
	}
}

func TestKindsFixedOrder(t *testing.T) {
	b := builder(t)
	lines, err := b.Kinds()
	if err != nil {
		t.Fatalf("Kinds 返回错误: %v", err)
	}
	all := model.AllDamageKinds()
	if len(lines) != len(all) {
		t.Fatalf("类型行数 = %d, 期望 %d", len(lines), len(all))
	}
	for i, k := range all {
		if lines[i].Kind != string(k) {
			t.Fatalf("第 %d 行类型 = %s, 期望 %s", i, lines[i].Kind, k)
		}
	}
	var claims int
	for _, l := range lines {
		claims += l.Claims
	}
	if claims != len(seed.Claims()) {
		t.Fatalf("类型案件数之和 = %d, 期望 %d", claims, len(seed.Claims()))
	}
}

func TestDocketWithoutMissingCounter(t *testing.T) {
	reg, err := seed.Load()
	if err != nil {
		t.Fatalf("seed.Load 失败: %v", err)
	}
	b := NewBuilder(reg, nil)
	rep, err := b.Docket(at())
	if err != nil {
		t.Fatalf("Docket 返回错误: %v", err)
	}
	for _, l := range rep.Lines {
		if l.MissingItems != 0 {
			t.Fatalf("未注入统计函数时 MissingItems 应为 0, 实际 %d", l.MissingItems)
		}
	}
}
