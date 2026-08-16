// Package gateway 封装对外部损害鉴定评估机构的调用。
//
// 鉴定评估是外部同步调用，响应时间不可控，因此每次调用都必须携带调用方
// 传入的 context：调用方的超时或取消必须能够立即终止在途请求，
// 不得在中间层替换为独立的、不会被取消的 context。
package gateway

import (
	"context"
	"fmt"
	"time"

	"ecoclaim/internal/model"
)

// Quote 是鉴定评估机构返回的报价。
type Quote struct {
	ClaimID    string    `json:"claim_id"`
	Institute  string    `json:"institute"`
	AmountYuan float64   `json:"amount_yuan"`
	IssuedAt   time.Time `json:"issued_at"`
}

// Client 是鉴定评估网关客户端。
type Client struct {
	institute string
	latency   time.Duration
	now       func() time.Time
}

// Options 是构造网关客户端的参数。
type Options struct {
	Institute string
	// Latency 模拟外部机构的响应耗时。
	Latency time.Duration
	Now     func() time.Time
}

// New 构造网关客户端。
func New(opts Options) *Client {
	institute := opts.Institute
	if institute == "" {
		institute = "省级生态环境损害司法鉴定中心"
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Client{institute: institute, latency: opts.Latency, now: now}
}

// Institute 返回鉴定机构名称。
func (c *Client) Institute() string {
	return c.institute
}

// call 执行一次外部请求：等待响应耗时，或在 ctx 结束时立即返回。
func (c *Client) call(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.latency <= 0 {
		return nil
	}
	timer := time.NewTimer(c.latency)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// RequestQuote 向鉴定评估机构申请报价。
//
// ctx 控制本次调用的生命周期：ctx 超时或被取消时必须立即返回对应错误，
// 不得继续等待外部机构响应。
func (c *Client) RequestQuote(ctx context.Context, claimID string, baseline float64) (Quote, error) {
	if claimID == "" {
		return Quote{}, fmt.Errorf("%w: 缺少案件编号", model.ErrInvalidClaim)
	}
	if err := c.call(ctx); err != nil {
		return Quote{}, fmt.Errorf("gateway: 案件 %s 鉴定评估调用未完成: %w", claimID, err)
	}
	return Quote{
		ClaimID:    claimID,
		Institute:  c.institute,
		AmountYuan: baseline,
		IssuedAt:   c.now(),
	}, nil
}

// Report 是鉴定评估报告。
type Report struct {
	ClaimID    string    `json:"claim_id"`
	Institute  string    `json:"institute"`
	Conclusion string    `json:"conclusion"`
	IssuedAt   time.Time `json:"issued_at"`
}

// FetchReport 拉取鉴定评估报告。
//
// 与 RequestQuote 相同，ctx 必须一路透传到外部调用。
func (c *Client) FetchReport(ctx context.Context, claimID string) (Report, error) {
	if claimID == "" {
		return Report{}, fmt.Errorf("%w: 缺少案件编号", model.ErrInvalidClaim)
	}
	if err := c.call(ctx); err != nil {
		return Report{}, fmt.Errorf("gateway: 案件 %s 鉴定报告拉取未完成: %w", claimID, err)
	}
	return Report{
		ClaimID:    claimID,
		Institute:  c.institute,
		Conclusion: "损害事实成立，具备量化条件",
		IssuedAt:   c.now(),
	}, nil
}

// Probe 探测网关可用性。
func (c *Client) Probe(ctx context.Context) error {
	if err := c.call(ctx); err != nil {
		return fmt.Errorf("%w: %s", model.ErrGatewayUnavailable, err.Error())
	}
	return nil
}
