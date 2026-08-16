// Package model 定义生态环境损害赔偿案件管理平台的领域模型。
//
// 平台面向生态环境损害赔偿制度改革实践，覆盖案件线索登记、损害类型认定、
// 磋商与诉讼时限管理、赔偿金额核算、生态修复工程跟踪与结案归档。
package model

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// DamageKind 表示生态环境损害类型。
type DamageKind string

const (
	// KindSoil 土壤污染。
	KindSoil DamageKind = "soil"
	// KindWater 水污染。
	KindWater DamageKind = "water"
	// KindAir 大气污染。
	KindAir DamageKind = "air"
	// KindNoise 噪声污染。
	KindNoise DamageKind = "noise"
	// KindSolidWaste 固体废物污染。
	KindSolidWaste DamageKind = "solid-waste"
	// KindEcology 生态破坏。
	KindEcology DamageKind = "ecology"
)

// AllDamageKinds 返回全部损害类型。
func AllDamageKinds() []DamageKind {
	return []DamageKind{KindSoil, KindWater, KindAir, KindNoise, KindSolidWaste, KindEcology}
}

// DisplayName 返回损害类型中文名。
func (k DamageKind) DisplayName() string {
	switch k {
	case KindSoil:
		return "土壤污染"
	case KindWater:
		return "水污染"
	case KindAir:
		return "大气污染"
	case KindNoise:
		return "噪声污染"
	case KindSolidWaste:
		return "固体废物污染"
	case KindEcology:
		return "生态破坏"
	default:
		return string(k)
	}
}

// ParseDamageKind 解析损害类型代码。
func ParseDamageKind(s string) (DamageKind, error) {
	v := DamageKind(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllDamageKinds() {
		if v == k {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownDamageKind, s)
}

// RestorationFeasible 报告该损害类型是否通常可以原地修复。
func (k DamageKind) RestorationFeasible() bool {
	return k == KindSoil || k == KindWater || k == KindEcology
}

// Stage 表示案件所处阶段。
type Stage string

const (
	// StageIntake 线索登记。
	StageIntake Stage = "intake"
	// StageInvestigation 调查取证。
	StageInvestigation Stage = "investigation"
	// StageAssessment 损害鉴定评估。
	StageAssessment Stage = "assessment"
	// StageNegotiation 磋商。
	StageNegotiation Stage = "negotiation"
	// StageAgreement 磋商达成协议。
	StageAgreement Stage = "agreement"
	// StageLitigation 提起诉讼。
	StageLitigation Stage = "litigation"
	// StageRestoration 修复实施。
	StageRestoration Stage = "restoration"
	// StageClosed 结案。
	StageClosed Stage = "closed"
	// StageTerminated 终止。
	StageTerminated Stage = "terminated"
)

// DisplayName 返回阶段中文名。
func (s Stage) DisplayName() string {
	switch s {
	case StageIntake:
		return "线索登记"
	case StageInvestigation:
		return "调查取证"
	case StageAssessment:
		return "损害鉴定评估"
	case StageNegotiation:
		return "磋商"
	case StageAgreement:
		return "磋商达成协议"
	case StageLitigation:
		return "提起诉讼"
	case StageRestoration:
		return "修复实施"
	case StageClosed:
		return "结案"
	case StageTerminated:
		return "终止"
	default:
		return string(s)
	}
}

// Terminal 报告该阶段是否为终态。
func (s Stage) Terminal() bool {
	return s == StageClosed || s == StageTerminated
}

// AllStages 返回全部阶段。
func AllStages() []Stage {
	return []Stage{
		StageIntake, StageInvestigation, StageAssessment, StageNegotiation,
		StageAgreement, StageLitigation, StageRestoration, StageClosed, StageTerminated,
	}
}

// ParseStage 解析阶段代码。
func ParseStage(s string) (Stage, error) {
	v := Stage(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range AllStages() {
		if v == k {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownStage, s)
}

// Severity 表示损害程度等级。
type Severity string

const (
	// SeverityMinor 轻微。
	SeverityMinor Severity = "minor"
	// SeverityModerate 较重。
	SeverityModerate Severity = "moderate"
	// SeveritySerious 严重。
	SeveritySerious Severity = "serious"
	// SeverityMajor 特别严重。
	SeverityMajor Severity = "major"
)

// DisplayName 返回损害程度中文名。
func (s Severity) DisplayName() string {
	switch s {
	case SeverityMinor:
		return "轻微"
	case SeverityModerate:
		return "较重"
	case SeveritySerious:
		return "严重"
	case SeverityMajor:
		return "特别严重"
	default:
		return string(s)
	}
}

// Multiplier 返回该程度对应的赔偿系数。
func (s Severity) Multiplier() float64 {
	switch s {
	case SeverityMinor:
		return 1.0
	case SeverityModerate:
		return 1.5
	case SeveritySerious:
		return 2.2
	case SeverityMajor:
		return 3.0
	default:
		return 1.0
	}
}

// ParseSeverity 解析损害程度代码。
func ParseSeverity(s string) (Severity, error) {
	v := Severity(strings.ToLower(strings.TrimSpace(s)))
	switch v {
	case SeverityMinor, SeverityModerate, SeveritySerious, SeverityMajor:
		return v, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownSeverity, s)
	}
}

// Respondent 表示赔偿义务人。
type Respondent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Province string `json:"province"`
}

// Claim 表示一件生态环境损害赔偿案件。
type Claim struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Kind          DamageKind   `json:"kind"`
	Severity      Severity     `json:"severity"`
	Stage         Stage        `json:"stage"`
	RespondentID  string       `json:"respondent_id"`
	Province      string       `json:"province"`
	FiledAt       time.Time    `json:"filed_at"`
	AffectedArea  float64      `json:"affected_area_mu"`
	AffectedDays  int          `json:"affected_days"`
	BaselineCost  float64      `json:"baseline_cost_yuan"`
	Restorable    bool         `json:"restorable"`
	AwardedAmount float64      `json:"awarded_amount_yuan"`
	Transitions   []Transition `json:"transitions"`
}

// Transition 记录一次阶段流转。
type Transition struct {
	From     Stage     `json:"from"`
	To       Stage     `json:"to"`
	At       time.Time `json:"at"`
	Operator string    `json:"operator"`
	Note     string    `json:"note"`
}

// Validate 校验案件登记信息。
func (c Claim) Validate() error {
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: 案件编号为空", ErrInvalidClaim)
	}
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("%w: 案件 %s 缺少案由", ErrInvalidClaim, c.ID)
	}
	if _, err := ParseDamageKind(string(c.Kind)); err != nil {
		return fmt.Errorf("%w: 案件 %s 损害类型非法", ErrInvalidClaim, c.ID)
	}
	if _, err := ParseSeverity(string(c.Severity)); err != nil {
		return fmt.Errorf("%w: 案件 %s 损害程度非法", ErrInvalidClaim, c.ID)
	}
	if strings.TrimSpace(c.RespondentID) == "" {
		return fmt.Errorf("%w: 案件 %s 缺少赔偿义务人", ErrInvalidClaim, c.ID)
	}
	if c.FiledAt.IsZero() {
		return fmt.Errorf("%w: 案件 %s 缺少立案日期", ErrInvalidClaim, c.ID)
	}
	if c.AffectedArea < 0 {
		return fmt.Errorf("%w: 案件 %s 受影响面积不得为负", ErrInvalidClaim, c.ID)
	}
	if c.AffectedDays < 0 {
		return fmt.Errorf("%w: 案件 %s 持续天数不得为负", ErrInvalidClaim, c.ID)
	}
	if c.BaselineCost < 0 {
		return fmt.Errorf("%w: 案件 %s 基准费用不得为负", ErrInvalidClaim, c.ID)
	}
	return nil
}

// LastTransition 返回最近一次阶段流转。
func (c Claim) LastTransition() (Transition, bool) {
	if len(c.Transitions) == 0 {
		return Transition{}, false
	}
	return c.Transitions[len(c.Transitions)-1], true
}

// SortClaims 按立案日期、编号排序，保证输出稳定。
func SortClaims(items []Claim) {
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].FiledAt.Equal(items[j].FiledAt) {
			return items[i].FiledAt.Before(items[j].FiledAt)
		}
		return items[i].ID < items[j].ID
	})
}
