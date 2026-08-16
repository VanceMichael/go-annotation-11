// Package assess 实现生态环境损害赔偿金额核算。
//
// 核算口径：
//
//	赔偿金额 = (清污费用 + 修复费用 + 期间损害 + 调查鉴定费用) * 损害程度系数
//
// 其中期间损害按受影响面积与持续天数计价；不可原地修复的案件按替代修复
// 计价，费率上浮。核算结果统一四舍五入到分。
package assess

import (
	"fmt"
	"math"

	"ecoclaim/internal/model"
)

// 各项费率常量，单位：元。
const (
	// CleanupRatePerMu 每亩清污费用。
	CleanupRatePerMu = 1800.0
	// RestorationRatePerMu 每亩原地修复费用。
	RestorationRatePerMu = 4200.0
	// AlternativeRatePerMu 每亩替代修复费用。
	AlternativeRatePerMu = 6800.0
	// InterimRatePerMuDay 每亩每日期间损害。
	InterimRatePerMuDay = 12.5
	// SurveyFeeFloor 调查鉴定费用下限。
	SurveyFeeFloor = 20000.0
	// SurveyFeeRate 调查鉴定费用占直接费用的比例。
	SurveyFeeRate = 0.06
)

// Breakdown 是赔偿金额核算明细。
type Breakdown struct {
	ClaimID     string  `json:"claim_id"`
	Cleanup     float64 `json:"cleanup_yuan"`
	Restoration float64 `json:"restoration_yuan"`
	Interim     float64 `json:"interim_yuan"`
	Survey      float64 `json:"survey_yuan"`
	Baseline    float64 `json:"baseline_yuan"`
	Multiplier  float64 `json:"multiplier"`
	Subtotal    float64 `json:"subtotal_yuan"`
	Total       float64 `json:"total_yuan"`
	Alternative bool    `json:"alternative_restoration"`
}

// Compute 核算一件案件的赔偿金额明细。
func Compute(c model.Claim) (Breakdown, error) {
	if err := c.Validate(); err != nil {
		return Breakdown{}, err
	}
	if _, err := model.ParseSeverity(string(c.Severity)); err != nil {
		return Breakdown{}, err
	}

	alternative := !c.Restorable
	restorationRate := RestorationRatePerMu
	if alternative {
		restorationRate = AlternativeRatePerMu
	}

	b := Breakdown{
		ClaimID:     c.ID,
		Cleanup:     round2(c.AffectedArea * CleanupRatePerMu),
		Restoration: round2(c.AffectedArea * restorationRate),
		Interim:     round2(c.AffectedArea * float64(c.AffectedDays) * InterimRatePerMuDay),
		Baseline:    round2(c.BaselineCost),
		Multiplier:  c.Severity.Multiplier(),
		Alternative: alternative,
	}

	direct := b.Cleanup + b.Restoration + b.Interim + b.Baseline
	b.Survey = round2(math.Max(SurveyFeeFloor, direct*SurveyFeeRate))
	b.Subtotal = round2(direct + b.Survey)
	b.Total = round2(b.Subtotal * b.Multiplier)
	return b, nil
}

// Summary 汇总一批核算结果。
type Summary struct {
	Claims     int     `json:"claims"`
	TotalYuan  float64 `json:"total_yuan"`
	MaxYuan    float64 `json:"max_yuan"`
	MinYuan    float64 `json:"min_yuan"`
	MeanYuan   float64 `json:"mean_yuan"`
	MaxClaimID string  `json:"max_claim_id"`
}

// Aggregate 汇总多件案件的核算结果。
func Aggregate(items []Breakdown) Summary {
	s := Summary{Claims: len(items)}
	if len(items) == 0 {
		return s
	}
	s.MinYuan = items[0].Total
	for _, b := range items {
		s.TotalYuan += b.Total
		if b.Total > s.MaxYuan {
			s.MaxYuan = b.Total
			s.MaxClaimID = b.ClaimID
		}
		if b.Total < s.MinYuan {
			s.MinYuan = b.Total
		}
	}
	s.TotalYuan = round2(s.TotalYuan)
	s.MeanYuan = round2(s.TotalYuan / float64(len(items)))
	return s
}

// Describe 返回核算明细的可读描述。
func (b Breakdown) Describe() string {
	mode := "原地修复"
	if b.Alternative {
		mode = "替代修复"
	}
	return fmt.Sprintf("案件 %s: %s, 直接费用 %.2f 元, 调查鉴定 %.2f 元, 系数 %.1f, 合计 %.2f 元",
		b.ClaimID, mode, b.Subtotal-b.Survey, b.Survey, b.Multiplier, b.Total)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
