package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"ecoclaim/internal/model"
)

func TestRequestQuoteFast(t *testing.T) {
	c := New(Options{Latency: time.Millisecond})
	q, err := c.RequestQuote(context.Background(), "HJ-1", 100000)
	if err != nil {
		t.Fatalf("RequestQuote 返回错误: %v", err)
	}
	if q.ClaimID != "HJ-1" || q.AmountYuan != 100000 {
		t.Fatalf("报价 = %+v", q)
	}
	if q.Institute == "" {
		t.Fatalf("缺少鉴定机构")
	}
}

// TestRequestQuoteHonoursTimeout 断言调用方超时后立即返回，不等待外部机构响应。
func TestRequestQuoteHonoursTimeout(t *testing.T) {
	c := New(Options{Latency: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	begin := time.Now()
	_, err := c.RequestQuote(ctx, "HJ-1", 100000)
	elapsed := time.Since(begin)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false, 错误为 %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("耗时 = %v, 期望远小于 1s（外部耗时 5s）", elapsed)
	}
}

// TestRequestQuoteHonoursCancel 断言调用方取消后立即返回。
func TestRequestQuoteHonoursCancel(t *testing.T) {
	c := New(Options{Latency: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	defer cancel()

	begin := time.Now()
	_, err := c.RequestQuote(ctx, "HJ-1", 100000)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("errors.Is(err, context.Canceled) = false, 错误为 %v", err)
	}
	if time.Since(begin) > time.Second {
		t.Fatalf("取消后未及时返回")
	}
}

func TestRequestQuoteAlreadyCancelled(t *testing.T) {
	c := New(Options{Latency: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.RequestQuote(ctx, "HJ-1", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("已取消应返回 context.Canceled, 实际 %v", err)
	}
}

// TestFetchReportHonoursTimeout 断言拉取报告同样尊重调用方超时。
func TestFetchReportHonoursTimeout(t *testing.T) {
	c := New(Options{Latency: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	begin := time.Now()
	_, err := c.FetchReport(ctx, "HJ-1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("errors.Is(err, context.DeadlineExceeded) = false, 错误为 %v", err)
	}
	if time.Since(begin) > time.Second {
		t.Fatalf("耗时过长")
	}
}

func TestFetchReportFast(t *testing.T) {
	c := New(Options{Latency: 0})
	r, err := c.FetchReport(context.Background(), "HJ-1")
	if err != nil {
		t.Fatalf("FetchReport 返回错误: %v", err)
	}
	if r.Conclusion == "" {
		t.Fatalf("报告结论为空")
	}
}

// TestProbeHonoursTimeout 断言探测尊重调用方超时并归类为网关不可用。
func TestProbeHonoursTimeout(t *testing.T) {
	c := New(Options{Latency: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	begin := time.Now()
	err := c.Probe(ctx)
	if err == nil {
		t.Fatalf("超时应返回错误")
	}
	if !errors.Is(err, model.ErrGatewayUnavailable) {
		t.Fatalf("errors.Is(err, model.ErrGatewayUnavailable) = false, 错误为 %v", err)
	}
	if time.Since(begin) > time.Second {
		t.Fatalf("耗时过长")
	}
}

func TestProbeOK(t *testing.T) {
	c := New(Options{})
	if err := c.Probe(context.Background()); err != nil {
		t.Fatalf("零耗时探测应成功: %v", err)
	}
}

func TestMissingClaimID(t *testing.T) {
	c := New(Options{})
	if _, err := c.RequestQuote(context.Background(), "", 1); !errors.Is(err, model.ErrInvalidClaim) {
		t.Fatalf("缺少案件编号应返回 ErrInvalidClaim, 实际 %v", err)
	}
	if _, err := c.FetchReport(context.Background(), ""); !errors.Is(err, model.ErrInvalidClaim) {
		t.Fatalf("缺少案件编号应返回 ErrInvalidClaim, 实际 %v", err)
	}
}

func TestInstituteDefault(t *testing.T) {
	if got := New(Options{}).Institute(); got == "" {
		t.Fatalf("默认鉴定机构为空")
	}
	if got := New(Options{Institute: "某中心"}).Institute(); got != "某中心" {
		t.Fatalf("Institute = %s", got)
	}
}

func TestNowOverride(t *testing.T) {
	fixed := time.Date(2026, time.August, 16, 0, 0, 0, 0, time.UTC)
	c := New(Options{Now: func() time.Time { return fixed }})
	q, err := c.RequestQuote(context.Background(), "HJ-1", 1)
	if err != nil {
		t.Fatalf("RequestQuote 返回错误: %v", err)
	}
	if !q.IssuedAt.Equal(fixed) {
		t.Fatalf("IssuedAt = %s", q.IssuedAt)
	}
}
