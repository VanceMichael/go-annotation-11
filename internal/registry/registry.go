// Package registry 维护案件与赔偿义务人的存储。
package registry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"ecoclaim/internal/model"
)

// Registry 是线程安全的案件登记簿。
type Registry struct {
	mu          sync.RWMutex
	claims      map[string]model.Claim
	respondents map[string]model.Respondent
}

// New 构造空登记簿。
func New() *Registry {
	return &Registry{
		claims:      make(map[string]model.Claim),
		respondents: make(map[string]model.Respondent),
	}
}

// AddRespondent 登记赔偿义务人。
func (r *Registry) AddRespondent(x model.Respondent) error {
	if strings.TrimSpace(x.ID) == "" {
		return fmt.Errorf("registry: 赔偿义务人编号为空")
	}
	if strings.TrimSpace(x.Name) == "" {
		return fmt.Errorf("registry: 赔偿义务人 %s 缺少名称", x.ID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.respondents[x.ID] = x
	return nil
}

// Respondent 返回赔偿义务人。
func (r *Registry) Respondent(id string) (model.Respondent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.respondents[id]
	if !ok {
		return model.Respondent{}, fmt.Errorf("%w: %s", model.ErrRespondentUnknown, id)
	}
	return x, nil
}

// Respondents 返回全部赔偿义务人，按编号排序。
func (r *Registry) Respondents() []model.Respondent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Respondent, 0, len(r.respondents))
	for _, x := range r.respondents {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Add 登记一件案件。
func (r *Registry) Add(c model.Claim) error {
	if err := c.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.claims[c.ID]; exists {
		return fmt.Errorf("%w: %s", model.ErrDuplicateClaim, c.ID)
	}
	if _, ok := r.respondents[c.RespondentID]; !ok {
		return fmt.Errorf("%w: 案件 %s 的赔偿义务人 %s", model.ErrRespondentUnknown, c.ID, c.RespondentID)
	}
	r.claims[c.ID] = c
	return nil
}

// Claim 返回案件。
func (r *Registry) Claim(id string) (model.Claim, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.claims[id]
	if !ok {
		return model.Claim{}, fmt.Errorf("%w: %s", model.ErrClaimUnknown, id)
	}
	return c, nil
}

// Save 覆盖保存案件。
func (r *Registry) Save(c model.Claim) error {
	if err := c.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.claims[c.ID]; !ok {
		return fmt.Errorf("%w: %s", model.ErrClaimUnknown, c.ID)
	}
	r.claims[c.ID] = c
	return nil
}

// Claims 返回全部案件，按立案日期与编号排序。
func (r *Registry) Claims() []model.Claim {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Claim, 0, len(r.claims))
	for _, c := range r.claims {
		out = append(out, c)
	}
	model.SortClaims(out)
	return out
}

// ClaimsByStage 返回指定阶段的案件。
func (r *Registry) ClaimsByStage(stage model.Stage) []model.Claim {
	all := r.Claims()
	out := make([]model.Claim, 0, len(all))
	for _, c := range all {
		if c.Stage == stage {
			out = append(out, c)
		}
	}
	return out
}

// ClaimsByKind 返回指定损害类型的案件。
func (r *Registry) ClaimsByKind(kind model.DamageKind) []model.Claim {
	all := r.Claims()
	out := make([]model.Claim, 0, len(all))
	for _, c := range all {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// IDs 返回全部案件编号，按字典序排序。
func (r *Registry) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.claims))
	for id := range r.claims {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Counts 汇总登记簿规模。
type Counts struct {
	Claims      int `json:"claims"`
	Respondents int `json:"respondents"`
	Closed      int `json:"closed"`
	Open        int `json:"open"`
}

// Counts 返回登记簿规模统计。
func (r *Registry) Counts() Counts {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c := Counts{Claims: len(r.claims), Respondents: len(r.respondents)}
	for _, cl := range r.claims {
		if cl.Stage.Terminal() {
			c.Closed++
		} else {
			c.Open++
		}
	}
	return c
}

// StageCounts 返回按阶段分组的案件数量。
func (r *Registry) StageCounts() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]int)
	for _, c := range r.claims {
		out[string(c.Stage)]++
	}
	return out
}
