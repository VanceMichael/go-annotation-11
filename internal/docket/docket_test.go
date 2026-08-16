package docket

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ecoclaim/internal/model"
	"ecoclaim/internal/seed"
)

func makeClaims(n int) []model.Claim {
	filed := time.Date(2026, time.March, 9, 0, 0, 0, 0, time.UTC)
	out := make([]model.Claim, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, model.Claim{
			ID:           fmt.Sprintf("HJ-BATCH-%03d", i+1),
			Title:        "批量核算测试案件",
			Kind:         model.KindSoil,
			Severity:     model.SeveritySerious,
			Stage:        model.StageRestoration,
			RespondentID: "R-001",
			Province:     "江苏",
			FiledAt:      filed,
			AffectedArea: float64(10 + i),
			AffectedDays: 100,
			BaselineCost: 200000,
			Restorable:   true,
			Transitions: []model.Transition{
				{From: model.StageIntake, To: model.StageInvestigation, At: filed, Operator: "测试"},
			},
		})
	}
	return out
}

// TestRunCoversEveryClaim 断言批量核算的结果覆盖全部案件。
func TestRunCoversEveryClaim(t *testing.T) {
	for _, n := range []int{1, 3, 7, 12} {
		claims := makeClaims(n)
		r := New(4, 3*time.Millisecond)
		res, err := r.Run(context.Background(), ItemsFrom(claims))
		if err != nil {
			t.Fatalf("n=%d: Run 返回错误: %v", n, err)
		}
		if res.Requested != n {
			t.Fatalf("n=%d: Requested = %d", n, res.Requested)
		}
		if len(res.Outcomes) != n {
			t.Fatalf("n=%d: 产出结果 %d 件, 期望 %d 件（不得在部分协程未完成时提前返回）",
				n, len(res.Outcomes), n)
		}
		if !res.Complete() {
			t.Fatalf("n=%d: Complete() = false, 报告 %+v", n, res)
		}
		if res.Completed != n {
			t.Fatalf("n=%d: Completed = %d, 期望 %d", n, res.Completed, n)
		}
	}
}

// TestRunCoversEveryClaimWithSettleDelay 断言单件核算存在耗时时结果仍然完整。
func TestRunCoversEveryClaimWithSettleDelay(t *testing.T) {
	claims := makeClaims(8)
	r := New(3, 30*time.Millisecond)
	res, err := r.Run(context.Background(), ItemsFrom(claims))
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if len(res.Outcomes) != 8 {
		t.Fatalf("产出结果 %d 件, 期望 8 件", len(res.Outcomes))
	}
	if res.Summary.Claims != 8 {
		t.Fatalf("汇总件数 = %d, 期望 8", res.Summary.Claims)
	}
	if res.Summary.TotalYuan <= 0 {
		t.Fatalf("汇总金额 = %.2f, 应为正", res.Summary.TotalYuan)
	}
}

// TestRunSummaryTotalsMatchOutcomes 断言汇总金额等于各件结果之和。
func TestRunSummaryTotalsMatchOutcomes(t *testing.T) {
	claims := makeClaims(10)
	r := New(4, 5*time.Millisecond)
	res, err := r.Run(context.Background(), ItemsFrom(claims))
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	var sum float64
	for _, o := range res.Outcomes {
		sum += o.TotalYuan
	}
	if len(res.Outcomes) != 10 {
		t.Fatalf("产出结果 %d 件, 期望 10 件", len(res.Outcomes))
	}
	if diff := sum - res.Summary.TotalYuan; diff > 0.01 || diff < -0.01 {
		t.Fatalf("结果之和 %.2f 与汇总 %.2f 不一致", sum, res.Summary.TotalYuan)
	}
}

// TestRunOutcomesSortedAndUnique 断言结果按案件编号排序且无重复无缺失。
func TestRunOutcomesSortedAndUnique(t *testing.T) {
	claims := makeClaims(9)
	r := New(5, 10*time.Millisecond)
	res, err := r.Run(context.Background(), ItemsFrom(claims))
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if len(res.Outcomes) != len(claims) {
		t.Fatalf("产出结果 %d 件, 期望 %d 件", len(res.Outcomes), len(claims))
	}
	seen := make(map[string]struct{}, len(res.Outcomes))
	for i, o := range res.Outcomes {
		if _, dup := seen[o.ClaimID]; dup {
			t.Fatalf("案件 %s 结果重复", o.ClaimID)
		}
		seen[o.ClaimID] = struct{}{}
		if i > 0 && res.Outcomes[i-1].ClaimID > o.ClaimID {
			t.Fatalf("结果未按案件编号排序: %+v", res.Outcomes)
		}
	}
	for _, c := range claims {
		if _, ok := seen[c.ID]; !ok {
			t.Fatalf("案件 %s 的结果缺失", c.ID)
		}
	}
}

// TestRunWithSeedClaims 断言内置样例案件全部产出结果。
func TestRunWithSeedClaims(t *testing.T) {
	claims := seed.Claims()
	r := New(4, 5*time.Millisecond)
	res, err := r.Run(context.Background(), ItemsFrom(claims))
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if len(res.Outcomes) != len(claims) {
		t.Fatalf("产出结果 %d 件, 期望 %d 件", len(res.Outcomes), len(claims))
	}
	if !res.Complete() {
		t.Fatalf("Complete() = false, 报告 %+v", res)
	}
}

func TestRunRejectsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := New(3, 0)
	if _, err := r.Run(ctx, ItemsFrom(makeClaims(4))); !errors.Is(err, context.Canceled) {
		t.Fatalf("启动前已取消应返回 context.Canceled, 实际 %v", err)
	}
}

func TestRunEmptyBatch(t *testing.T) {
	r := New(3, 0)
	res, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("空批次不应返回错误: %v", err)
	}
	if res.Requested != 0 || !res.Complete() {
		t.Fatalf("报告 = %+v", res)
	}
}

func TestRunRecordsFailures(t *testing.T) {
	claims := makeClaims(3)
	claims[1].Severity = model.Severity("unknown") // 触发核算失败
	r := New(2, 3*time.Millisecond)
	res, err := r.Run(context.Background(), ItemsFrom(claims))
	if err != nil {
		t.Fatalf("单件失败不应导致整批出错: %v", err)
	}
	if len(res.Outcomes) != 3 {
		t.Fatalf("产出结果 %d 件, 期望 3 件", len(res.Outcomes))
	}
	if res.Failed != 1 || res.Completed != 2 {
		t.Fatalf("报告 = %+v, 期望 failed=1 completed=2", res)
	}
	if !res.Complete() {
		t.Fatalf("Complete() 应为 true")
	}
}

func TestRunHonoursTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	r := New(2, 3*time.Second)
	begin := time.Now()
	res, err := r.Run(ctx, ItemsFrom(makeClaims(6)))
	if err != nil {
		t.Fatalf("Run 返回错误: %v", err)
	}
	if time.Since(begin) > 2*time.Second {
		t.Fatalf("超时后耗时 = %v, 期望远小于 3s", time.Since(begin))
	}
	if len(res.Outcomes) != 6 {
		t.Fatalf("产出结果 %d 件, 期望 6 件（超时也应为每件案件产出结果）", len(res.Outcomes))
	}
	if res.Failed == 0 {
		t.Fatalf("超时下应记录失败结果, 报告 %+v", res)
	}
}

func TestNewClampsWorkers(t *testing.T) {
	if got := New(0, 0).Workers(); got != 1 {
		t.Fatalf("workers = %d, 期望 1", got)
	}
	if got := New(-3, 0).Workers(); got != 1 {
		t.Fatalf("workers = %d, 期望 1", got)
	}
}
