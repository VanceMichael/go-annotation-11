// Package seed 提供内置的赔偿义务人与案件样例数据。
//
// 样例取材于生态环境损害赔偿制度改革的典型案件类型，覆盖土壤、水、大气、
// 噪声、固废与生态破坏六类损害，仅用于本地演练。
package seed

import (
	"fmt"
	"time"

	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Respondents 返回内置赔偿义务人。
func Respondents() []model.Respondent {
	return []model.Respondent{
		{ID: "R-001", Name: "某化工有限公司", Kind: "企业", Province: "江苏"},
		{ID: "R-002", Name: "某矿业开发有限公司", Kind: "企业", Province: "云南"},
		{ID: "R-003", Name: "某商铺经营者", Kind: "个体", Province: "浙江"},
		{ID: "R-004", Name: "某养殖专业合作社", Kind: "合作社", Province: "湖南"},
		{ID: "R-005", Name: "某建材加工厂", Kind: "企业", Province: "河北"},
		{ID: "R-006", Name: "某固废处置公司", Kind: "企业", Province: "广东"},
	}
}

// Claims 返回内置案件。
func Claims() []model.Claim {
	rows := []struct {
		id           string
		title        string
		kind         model.DamageKind
		severity     model.Severity
		stage        model.Stage
		respondentID string
		province     string
		filed        time.Time
		area         float64
		days         int
		baseline     float64
		restorable   bool
		awarded      float64
		transitions  int
	}{
		{"HJ-2026-001", "非法排放含镍废水致农田土壤污染案", model.KindSoil, model.SeveritySerious,
			model.StageRestoration, "R-001", "江苏", day(2026, time.March, 9), 86.5, 142, 380000, true, 0, 4},
		{"HJ-2026-002", "尾矿库渗漏致河道水质超标案", model.KindWater, model.SeverityMajor,
			model.StageRestoration, "R-002", "云南", day(2026, time.April, 13), 210.0, 96, 1250000, true, 0, 4},
		{"HJ-2026-003", "商铺低频噪声长期扰民案", model.KindNoise, model.SeverityMinor,
			model.StageRestoration, "R-003", "浙江", day(2026, time.May, 18), 0, 210, 42000, false, 0, 4},
		{"HJ-2026-004", "违规投饵致湖泊水体富营养化案", model.KindWater, model.SeverityModerate,
			model.StageNegotiation, "R-004", "湖南", day(2026, time.June, 1), 340.0, 68, 260000, true, 0, 3},
		{"HJ-2026-005", "露天堆场扬尘致大气污染案", model.KindAir, model.SeverityModerate,
			model.StageAssessment, "R-005", "河北", day(2026, time.June, 22), 55.0, 120, 180000, false, 0, 2},
		{"HJ-2026-006", "非法转移倾倒危险废物案", model.KindSolidWaste, model.SeverityMajor,
			model.StageInvestigation, "R-006", "广东", day(2026, time.July, 6), 18.0, 45, 960000, false, 0, 1},
		{"HJ-2026-007", "非法采伐防护林致生态破坏案", model.KindEcology, model.SeveritySerious,
			model.StageIntake, "R-002", "云南", day(2026, time.July, 27), 128.0, 30, 420000, true, 0, 0},
	}

	out := make([]model.Claim, 0, len(rows))
	for _, r := range rows {
		c := model.Claim{
			ID:            r.id,
			Title:         r.title,
			Kind:          r.kind,
			Severity:      r.severity,
			Stage:         r.stage,
			RespondentID:  r.respondentID,
			Province:      r.province,
			FiledAt:       r.filed,
			AffectedArea:  r.area,
			AffectedDays:  r.days,
			BaselineCost:  r.baseline,
			Restorable:    r.restorable,
			AwardedAmount: r.awarded,
		}
		c.Transitions = buildTransitions(c, r.transitions)
		out = append(out, c)
	}
	return out
}

// buildTransitions 依据阶段数构造流转记录链。
func buildTransitions(c model.Claim, n int) []model.Transition {
	if n <= 0 {
		return nil
	}
	chain := []model.Stage{
		model.StageIntake, model.StageInvestigation, model.StageAssessment,
		model.StageNegotiation, model.StageAgreement, model.StageRestoration,
	}
	out := make([]model.Transition, 0, n)
	at := c.FiledAt
	for i := 0; i < n && i+1 < len(chain); i++ {
		at = at.AddDate(0, 0, 7)
		out = append(out, model.Transition{
			From:     chain[i],
			To:       chain[i+1],
			At:       at,
			Operator: "赔偿权利人指定部门",
			Note:     chain[i+1].DisplayName(),
		})
	}
	return out
}

// RestorationClaimIDs 返回处于修复实施阶段的案件编号。
func RestorationClaimIDs() []string {
	var out []string
	for _, c := range Claims() {
		if c.Stage == model.StageRestoration {
			out = append(out, c.ID)
		}
	}
	return out
}

// Load 构造带内置样例数据的登记簿。
func Load() (*registry.Registry, error) {
	reg := registry.New()
	for _, x := range Respondents() {
		if err := reg.AddRespondent(x); err != nil {
			return nil, fmt.Errorf("seed: 登记赔偿义务人 %s 失败: %w", x.ID, err)
		}
	}
	for _, c := range Claims() {
		if err := reg.Add(c); err != nil {
			return nil, fmt.Errorf("seed: 登记案件 %s 失败: %w", c.ID, err)
		}
	}
	return reg, nil
}
