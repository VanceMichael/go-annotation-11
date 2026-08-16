// Package claim 实现生态环境损害赔偿案件的阶段流转与结案服务。
package claim

import (
	"context"
	"fmt"
	"time"

	"ecoclaim/internal/assess"
	"ecoclaim/internal/deadline"
	"ecoclaim/internal/gateway"
	"ecoclaim/internal/model"
	"ecoclaim/internal/registry"
)

// transition 描述一条合法的阶段流转边。
type edge struct {
	from model.Stage
	to   model.Stage
}

var edges = map[edge]bool{
	{model.StageIntake, model.StageInvestigation}:     true,
	{model.StageIntake, model.StageTerminated}:        true,
	{model.StageInvestigation, model.StageAssessment}: true,
	{model.StageInvestigation, model.StageTerminated}: true,
	{model.StageAssessment, model.StageNegotiation}:   true,
	{model.StageAssessment, model.StageTerminated}:    true,
	{model.StageNegotiation, model.StageAgreement}:    true,
	{model.StageNegotiation, model.StageLitigation}:   true,
	{model.StageNegotiation, model.StageTerminated}:   true,
	{model.StageAgreement, model.StageRestoration}:    true,
	{model.StageLitigation, model.StageRestoration}:   true,
	{model.StageLitigation, model.StageTerminated}:    true,
	{model.StageRestoration, model.StageClosed}:       true,
	{model.StageRestoration, model.StageTerminated}:   true,
}

// Service 提供案件阶段流转、金额核定与结案能力。
type Service struct {
	registry *registry.Registry
	gateway  *gateway.Client
}

// NewService 构造案件服务。
func NewService(reg *registry.Registry, gw *gateway.Client) *Service {
	return &Service{registry: reg, gateway: gw}
}

// Allowed 返回某阶段允许流转到的目标阶段。
func Allowed(from model.Stage) []model.Stage {
	out := make([]model.Stage, 0, 3)
	for _, to := range model.AllStages() {
		if edges[edge{from, to}] {
			out = append(out, to)
		}
	}
	return out
}

// transition 在案件上执行一次阶段流转并追加记录。
func (s *Service) transition(c model.Claim, to model.Stage, operator, note string, at time.Time) (model.Claim, error) {
	if !edges[edge{c.Stage, to}] {
		return c, fmt.Errorf("%w: 案件 %s 不能从 %s 流转到 %s",
			model.ErrStageConflict, c.ID, c.Stage.DisplayName(), to.DisplayName())
	}
	next := c
	next.Stage = to
	history := make([]model.Transition, 0, len(c.Transitions)+1)
	history = append(history, c.Transitions...)
	history = append(history, model.Transition{
		From:     c.Stage,
		To:       to,
		At:       at,
		Operator: operator,
		Note:     note,
	})
	next.Transitions = history
	return next, nil
}

// Advance 推进案件到目标阶段。
func (s *Service) Advance(id string, to model.Stage, operator, note string, at time.Time) (model.Claim, error) {
	c, err := s.registry.Claim(id)
	if err != nil {
		return model.Claim{}, err
	}
	schedule, err := deadline.Compute(c.ID, c.FiledAt)
	if err != nil {
		return model.Claim{}, err
	}
	if derr := schedule.Check(at); derr != nil {
		return model.Claim{}, derr
	}
	next, err := s.transition(c, to, operator, note, at)
	if err != nil {
		return model.Claim{}, err
	}
	if err := s.registry.Save(next); err != nil {
		return model.Claim{}, err
	}
	return next, nil
}

// Assess 核定案件赔偿金额，必要时向外部鉴定机构取报价。
func (s *Service) Assess(ctx context.Context, id string) (assess.Breakdown, error) {
	c, err := s.registry.Claim(id)
	if err != nil {
		return assess.Breakdown{}, err
	}
	if c.Stage == model.StageIntake {
		return assess.Breakdown{}, fmt.Errorf("%w: 案件 %s 仍处于 %s",
			model.ErrAssessmentIncomplete, c.ID, c.Stage.DisplayName())
	}

	breakdown, err := assess.Compute(c)
	if err != nil {
		return assess.Breakdown{}, err
	}

	if s.gateway != nil {
		quote, qerr := s.gateway.RequestQuote(ctx, c.ID, breakdown.Baseline)
		if qerr != nil {
			return assess.Breakdown{}, qerr
		}
		_ = quote
	}

	c.AwardedAmount = breakdown.Total
	if err := s.registry.Save(c); err != nil {
		return assess.Breakdown{}, err
	}
	return breakdown, nil
}

// precheck 结案前置校验：材料齐备、时限未届满、金额已核定。
func (s *Service) precheck(c model.Claim, at time.Time) error {
	if verr := Validate(c); verr != nil {
		return fmt.Errorf("claim: 案件 %s 结案前置校验未通过: %w", c.ID, verr)
	}
	schedule, err := deadline.Compute(c.ID, c.FiledAt)
	if err != nil {
		return err
	}
	if derr := schedule.Check(at); derr != nil {
		return derr
	}
	if c.AwardedAmount <= 0 {
		return fmt.Errorf("%w: 案件 %s 尚未核定赔偿金额", model.ErrAssessmentIncomplete, c.ID)
	}
	return nil
}

// Finalize 结案。
//
// 前置校验不通过时必须返回对应错误，且案件阶段保持不变；
// 校验通过时把案件推进到结案阶段并持久化。
func (s *Service) Finalize(id string, operator string, at time.Time) error {
	c, err := s.registry.Claim(id)
	if err != nil {
		return err
	}
	if perr := s.precheck(c, at); perr != nil {
		return perr
	}
	next, terr := s.transition(c, model.StageClosed, operator, "结案", at)
	if terr != nil {
		return terr
	}
	return s.registry.Save(next)
}

// Schedule 返回案件时限表。
func (s *Service) Schedule(id string) (deadline.Schedule, error) {
	c, err := s.registry.Claim(id)
	if err != nil {
		return deadline.Schedule{}, err
	}
	return deadline.Compute(c.ID, c.FiledAt)
}

// Probe 探测外部鉴定评估网关。
func (s *Service) Probe(ctx context.Context) error {
	if s.gateway == nil {
		return nil
	}
	return s.gateway.Probe(ctx)
}
