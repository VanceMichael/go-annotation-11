// Package report 生成生态环境损害赔偿案件的统计报表。
package report

import (
	"fmt"
	"sort"
	"time"

	"ecoclaim/internal/assess"
	"ecoclaim/internal/deadline"
	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
)

// ClaimLine 是案件维度的报表行。
type ClaimLine struct {
	ClaimID        string  `json:"claim_id"`
	Title          string  `json:"title"`
	Kind           string  `json:"kind"`
	KindName       string  `json:"kind_name"`
	Severity       string  `json:"severity"`
	Stage          string  `json:"stage"`
	StageName      string  `json:"stage_name"`
	Province       string  `json:"province"`
	Respondent     string  `json:"respondent"`
	FiledAt        string  `json:"filed_at"`
	NoticeDue      string  `json:"notice_due"`
	NoticeOverdue  bool    `json:"notice_overdue"`
	AssessedYuan   float64 `json:"assessed_yuan"`
	MissingItems   int     `json:"missing_items"`
	LitigationOpen bool    `json:"litigation_open"`
}

// Docket 是案件台账报表。
type Docket struct {
	At          string         `json:"at"`
	Lines       []ClaimLine    `json:"lines"`
	StageCounts map[string]int `json:"stage_counts"`
	KindCounts  map[string]int `json:"kind_counts"`
	Summary     assess.Summary `json:"summary"`
	Overdue     int            `json:"overdue"`
}

// missingCounter 由 claim 包注入，避免 report 依赖 claim 造成循环引用。
type missingCounter func(model.Claim) int

// Builder 组装报表所需的数据源。
type Builder struct {
	registry *registry.Registry
	missing  missingCounter
}

// NewBuilder 构造报表生成器。missing 用于统计结案材料缺失项数量，可为 nil。
func NewBuilder(reg *registry.Registry, missing func(model.Claim) int) *Builder {
	return &Builder{registry: reg, missing: missing}
}

// Docket 生成案件台账报表。
func (b *Builder) Docket(at time.Time) (Docket, error) {
	out := Docket{
		At:          at.Format(time.RFC3339),
		StageCounts: map[string]int{},
		KindCounts:  map[string]int{},
	}
	breakdowns := make([]assess.Breakdown, 0, 8)

	for _, c := range b.registry.Claims() {
		schedule, err := deadline.Compute(c.ID, c.FiledAt)
		if err != nil {
			return Docket{}, fmt.Errorf("report: 案件 %s 时限计算失败: %w", c.ID, err)
		}
		line := ClaimLine{
			ClaimID:        c.ID,
			Title:          c.Title,
			Kind:           string(c.Kind),
			KindName:       c.Kind.DisplayName(),
			Severity:       string(c.Severity),
			Stage:          string(c.Stage),
			StageName:      c.Stage.DisplayName(),
			Province:       c.Province,
			FiledAt:        c.FiledAt.Format("2006-01-02"),
			NoticeDue:      schedule.NoticeDue.Format("2006-01-02"),
			NoticeOverdue:  schedule.NoticeOverdue(at),
			LitigationOpen: !schedule.LitigationBarred(at),
		}
		if x, rerr := b.registry.Respondent(c.RespondentID); rerr == nil {
			line.Respondent = x.Name
		}
		if b.missing != nil {
			line.MissingItems = b.missing(c)
		}
		if bd, aerr := assess.Compute(c); aerr == nil {
			line.AssessedYuan = bd.Total
			breakdowns = append(breakdowns, bd)
		}
		if line.NoticeOverdue {
			out.Overdue++
		}
		out.StageCounts[string(c.Stage)]++
		out.KindCounts[string(c.Kind)]++
		out.Lines = append(out.Lines, line)
	}

	sort.Slice(out.Lines, func(i, j int) bool { return out.Lines[i].ClaimID < out.Lines[j].ClaimID })
	out.Summary = assess.Aggregate(breakdowns)
	return out, nil
}

// ProvinceLine 是省份维度的报表行。
type ProvinceLine struct {
	Province     string  `json:"province"`
	Claims       int     `json:"claims"`
	Closed       int     `json:"closed"`
	AssessedYuan float64 `json:"assessed_yuan"`
}

// Provinces 生成省份维度报表，按赔偿金额降序。
func (b *Builder) Provinces() ([]ProvinceLine, error) {
	byProvince := make(map[string]*ProvinceLine)
	for _, c := range b.registry.Claims() {
		line, ok := byProvince[c.Province]
		if !ok {
			line = &ProvinceLine{Province: c.Province}
			byProvince[c.Province] = line
		}
		line.Claims++
		if c.Stage.Terminal() {
			line.Closed++
		}
		if bd, err := assess.Compute(c); err == nil {
			line.AssessedYuan += bd.Total
		}
	}
	out := make([]ProvinceLine, 0, len(byProvince))
	for _, line := range byProvince {
		out = append(out, *line)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].AssessedYuan != out[j].AssessedYuan {
			return out[i].AssessedYuan > out[j].AssessedYuan
		}
		return out[i].Province < out[j].Province
	})
	return out, nil
}

// KindLine 是损害类型维度的报表行。
type KindLine struct {
	Kind         string  `json:"kind"`
	KindName     string  `json:"kind_name"`
	Claims       int     `json:"claims"`
	Restorable   int     `json:"restorable"`
	AssessedYuan float64 `json:"assessed_yuan"`
}

// Kinds 生成损害类型维度报表，顺序固定。
func (b *Builder) Kinds() ([]KindLine, error) {
	byKind := make(map[model.DamageKind]*KindLine)
	for _, k := range model.AllDamageKinds() {
		byKind[k] = &KindLine{Kind: string(k), KindName: k.DisplayName()}
	}
	for _, c := range b.registry.Claims() {
		line, ok := byKind[c.Kind]
		if !ok {
			continue
		}
		line.Claims++
		if c.Restorable {
			line.Restorable++
		}
		if bd, err := assess.Compute(c); err == nil {
			line.AssessedYuan += bd.Total
		}
	}
	out := make([]KindLine, 0, len(byKind))
	for _, k := range model.AllDamageKinds() {
		out = append(out, *byKind[k])
	}
	return out, nil
}
