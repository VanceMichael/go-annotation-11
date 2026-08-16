// Package deadline 计算生态环境损害赔偿案件的法定时限。
//
// 平台涉及两类时限口径：
//
//	工作日口径：跳过周六、周日。起始日本身不计入，
//	            即 AddWorkdays(start, 1) 表示 start 之后的第一个工作日。
//	自然日口径：直接按日历天数推进。
//
// 各环节时限：
//
//	磋商启动告知      立案后 20 个工作日
//	鉴定评估报告提交  进入评估阶段后 60 个自然日
//	磋商期限          磋商启动后 90 个自然日
//	诉讼时效          损害发现之日起 3 年
package deadline

import (
	"fmt"
	"time"

	"ecoclaim/internal/model"
)

// 各环节时限常量。
const (
	// NoticeWorkdays 磋商启动告知期限，单位工作日。
	NoticeWorkdays = 20
	// AssessmentDays 鉴定评估报告提交期限，单位自然日。
	AssessmentDays = 60
	// NegotiationDays 磋商期限，单位自然日。
	NegotiationDays = 90
	// LitigationYears 诉讼时效，单位年。
	LitigationYears = 3
)

// IsWorkday 报告某日是否为工作日（周一至周五）。
func IsWorkday(t time.Time) bool {
	switch t.Weekday() {
	case time.Saturday, time.Sunday:
		return false
	default:
		return true
	}
}

// AddWorkdays 返回 start 之后第 n 个工作日。
//
// 起始日本身不计入：即使 start 是工作日，也从 start 的次日开始计数。
// n 不为正时直接返回 start。
func AddWorkdays(start time.Time, n int) time.Time {
	if n <= 0 {
		return start
	}
	d := start
	added := 0
	for added < n {
		d = d.AddDate(0, 0, 1)
		if IsWorkday(d) {
			added++
		}
	}
	return d
}

// WorkdaysBetween 返回 from 之后到 to（含）之间的工作日天数。
// to 不晚于 from 时返回 0。
func WorkdaysBetween(from, to time.Time) int {
	if !to.After(from) {
		return 0
	}
	count := 0
	d := from
	for {
		d = d.AddDate(0, 0, 1)
		if d.After(to) {
			break
		}
		if IsWorkday(d) {
			count++
		}
	}
	return count
}

// Schedule 是一件案件的时限表。
type Schedule struct {
	ClaimID            string    `json:"claim_id"`
	FiledAt            time.Time `json:"filed_at"`
	NoticeDue          time.Time `json:"notice_due"`
	AssessmentDue      time.Time `json:"assessment_due"`
	NegotiationDue     time.Time `json:"negotiation_due"`
	LitigationBarredAt time.Time `json:"litigation_barred_at"`
}

// Compute 依据立案日期生成案件时限表。
func Compute(claimID string, filedAt time.Time) (Schedule, error) {
	if filedAt.IsZero() {
		return Schedule{}, fmt.Errorf("%w: 案件 %s 缺少立案日期", model.ErrInvalidClaim, claimID)
	}
	notice := AddWorkdays(filedAt, NoticeWorkdays)
	return Schedule{
		ClaimID:            claimID,
		FiledAt:            filedAt,
		NoticeDue:          notice,
		AssessmentDue:      filedAt.AddDate(0, 0, AssessmentDays),
		NegotiationDue:     notice.AddDate(0, 0, NegotiationDays),
		LitigationBarredAt: filedAt.AddDate(LitigationYears, 0, 0),
	}, nil
}

// NoticeOverdue 报告在 at 时刻磋商启动告知是否已超期。
func (s Schedule) NoticeOverdue(at time.Time) bool {
	return at.After(s.NoticeDue)
}

// AssessmentOverdue 报告在 at 时刻鉴定评估报告提交是否已超期。
func (s Schedule) AssessmentOverdue(at time.Time) bool {
	return at.After(s.AssessmentDue)
}

// NegotiationOverdue 报告在 at 时刻磋商期限是否已届满。
func (s Schedule) NegotiationOverdue(at time.Time) bool {
	return at.After(s.NegotiationDue)
}

// LitigationBarred 报告在 at 时刻诉讼时效是否已届满。
func (s Schedule) LitigationBarred(at time.Time) bool {
	return at.After(s.LitigationBarredAt)
}

// Check 在 at 时刻校验时限，已届满诉讼时效时返回 model.ErrDeadlineExpired。
func (s Schedule) Check(at time.Time) error {
	if s.LitigationBarred(at) {
		return fmt.Errorf("%w: 案件 %s 诉讼时效于 %s 届满",
			model.ErrDeadlineExpired, s.ClaimID, s.LitigationBarredAt.Format("2006-01-02"))
	}
	return nil
}

// RemainingWorkdaysForNotice 返回距磋商启动告知期限还剩的工作日天数。
func (s Schedule) RemainingWorkdaysForNotice(at time.Time) int {
	return WorkdaysBetween(at, s.NoticeDue)
}
