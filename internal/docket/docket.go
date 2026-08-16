// Package docket 实现案件批量核算。
//
// 批量核算把多件案件分发给并发工作协程处理，全部完成后统一汇总。
// 汇总必须覆盖全部案件：等待逻辑要保证在所有工作协程结束之后才返回，
// 不得在部分协程尚未完成时提前收集结果。
package docket

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"ecoclaim/internal/assess"
	"ecoclaim/internal/model"
)

// Item 是一件待核算案件。
type Item struct {
	Claim model.Claim
}

// Outcome 是单件案件的核算结果。
type Outcome struct {
	ClaimID    string  `json:"claim_id"`
	TotalYuan  float64 `json:"total_yuan"`
	Multiplier float64 `json:"multiplier"`
	Failed     bool    `json:"failed"`
	Message    string  `json:"message"`
}

// Result 是一批核算的汇总结果。
type Result struct {
	Requested int            `json:"requested"`
	Completed int            `json:"completed"`
	Failed    int            `json:"failed"`
	Outcomes  []Outcome      `json:"outcomes"`
	Summary   assess.Summary `json:"summary"`
	Elapsed   time.Duration  `json:"elapsed_ns"`
	Workers   int            `json:"workers"`
}

// Complete 报告是否所有请求的案件都产出了结果。
func (r Result) Complete() bool {
	return r.Completed+r.Failed == r.Requested
}

// Runner 并发执行批量核算。
type Runner struct {
	workers int
	// settle 模拟单件案件核算的外部耗时（鉴定机构接口）。
	settle time.Duration
}

// New 构造批量核算器。workers 小于 1 时按 1 处理。
func New(workers int, settle time.Duration) *Runner {
	if workers < 1 {
		workers = 1
	}
	return &Runner{workers: workers, settle: settle}
}

// Workers 返回并发度。
func (r *Runner) Workers() int {
	return r.workers
}

// Run 并发核算全部案件，返回覆盖所有案件的汇总结果。
//
// 返回前必须等待所有工作协程结束，Result.Outcomes 的条数
// 必须等于传入的案件件数。
func (r *Runner) Run(ctx context.Context, items []Item) (Result, error) {
	res := Result{Requested: len(items), Workers: r.workers}
	if err := ctx.Err(); err != nil {
		return res, fmt.Errorf("docket: 批量核算启动前已被取消: %w", err)
	}
	if len(items) == 0 {
		return res, nil
	}

	begin := time.Now()
	var mu sync.Mutex
	var wg sync.WaitGroup
	collected := make([]Outcome, 0, len(items))
	sem := make(chan struct{}, r.workers)

	for _, it := range items {
		wg.Add(1)
		go func(it Item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			outcome := r.settleOne(ctx, it)

			mu.Lock()
			collected = append(collected, outcome)
			mu.Unlock()
		}(it)
	}
	wg.Wait()

	mu.Lock()
	res.Outcomes = append(res.Outcomes, collected...)
	mu.Unlock()

	sort.Slice(res.Outcomes, func(i, j int) bool { return res.Outcomes[i].ClaimID < res.Outcomes[j].ClaimID })

	breakdowns := make([]assess.Breakdown, 0, len(res.Outcomes))
	for _, o := range res.Outcomes {
		if o.Failed {
			res.Failed++
			continue
		}
		res.Completed++
		breakdowns = append(breakdowns, assess.Breakdown{ClaimID: o.ClaimID, Total: o.TotalYuan, Multiplier: o.Multiplier})
	}
	res.Summary = assess.Aggregate(breakdowns)
	res.Elapsed = time.Since(begin)
	return res, nil
}

// settleOne 核算单件案件，先模拟外部耗时再计算金额。
func (r *Runner) settleOne(ctx context.Context, it Item) Outcome {
	if r.settle > 0 {
		timer := time.NewTimer(r.settle)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Outcome{ClaimID: it.Claim.ID, Failed: true, Message: ctx.Err().Error()}
		case <-timer.C:
		}
	}
	b, err := assess.Compute(it.Claim)
	if err != nil {
		return Outcome{ClaimID: it.Claim.ID, Failed: true, Message: err.Error()}
	}
	return Outcome{ClaimID: b.ClaimID, TotalYuan: b.Total, Multiplier: b.Multiplier}
}

// ItemsFrom 把案件列表包装为待核算条目。
func ItemsFrom(claims []model.Claim) []Item {
	out := make([]Item, 0, len(claims))
	for _, c := range claims {
		out = append(out, Item{Claim: c})
	}
	return out
}
