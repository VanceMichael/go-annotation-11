package deadline

import (
	"errors"
	"testing"
	"time"

	"ecoclaim/internal/model"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestIsWorkday(t *testing.T) {
	// 2026-08-17 是周一，08-22 周六，08-23 周日。
	cases := map[time.Time]bool{
		day(2026, time.August, 17): true,
		day(2026, time.August, 18): true,
		day(2026, time.August, 21): true,
		day(2026, time.August, 22): false,
		day(2026, time.August, 23): false,
		day(2026, time.August, 24): true,
	}
	for d, want := range cases {
		if got := IsWorkday(d); got != want {
			t.Errorf("%s (%s): IsWorkday = %v, 期望 %v", d.Format("2006-01-02"), d.Weekday(), got, want)
		}
	}
}

// TestAddWorkdaysExcludesStartDay 断言起始日本身不计入工作日计数。
func TestAddWorkdaysExcludesStartDay(t *testing.T) {
	// 2026-08-17 周一起算，第 1 个工作日应为 08-18 周二。
	start := day(2026, time.August, 17)
	cases := []struct {
		n    int
		want time.Time
	}{
		{1, day(2026, time.August, 18)},
		{2, day(2026, time.August, 19)},
		{3, day(2026, time.August, 20)},
		{4, day(2026, time.August, 21)},
		{5, day(2026, time.August, 24)}, // 跳过 22、23 周末
		{10, day(2026, time.August, 31)},
	}
	for _, tc := range cases {
		got := AddWorkdays(start, tc.n)
		if !got.Equal(tc.want) {
			t.Errorf("AddWorkdays(2026-08-17, %d) = %s, 期望 %s",
				tc.n, got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

// TestAddWorkdaysFromWeekend 断言起始日为周末时的计数同样从次日开始。
func TestAddWorkdaysFromWeekend(t *testing.T) {
	// 2026-08-22 周六起算，第 1 个工作日应为 08-24 周一。
	start := day(2026, time.August, 22)
	cases := []struct {
		n    int
		want time.Time
	}{
		{1, day(2026, time.August, 24)},
		{2, day(2026, time.August, 25)},
		{5, day(2026, time.August, 28)},
	}
	for _, tc := range cases {
		got := AddWorkdays(start, tc.n)
		if !got.Equal(tc.want) {
			t.Errorf("AddWorkdays(2026-08-22, %d) = %s, 期望 %s",
				tc.n, got.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

func TestAddWorkdaysNonPositive(t *testing.T) {
	start := day(2026, time.August, 17)
	for _, n := range []int{0, -1, -20} {
		if got := AddWorkdays(start, n); !got.Equal(start) {
			t.Errorf("AddWorkdays(start, %d) = %s, 期望原值", n, got.Format("2006-01-02"))
		}
	}
}

// TestNoticeDueIsTwentyWorkdaysAfterFiling 断言磋商启动告知期限为立案后 20 个工作日。
func TestNoticeDueIsTwentyWorkdaysAfterFiling(t *testing.T) {
	// 2026-08-17 周一立案，20 个工作日后为 2026-09-14 周一。
	s, err := Compute("HJ-2026-001", day(2026, time.August, 17))
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	want := day(2026, time.September, 14)
	if !s.NoticeDue.Equal(want) {
		t.Fatalf("磋商启动告知期限 = %s, 期望 %s",
			s.NoticeDue.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

// TestNoticeDueAcrossFilingWeekdays 断言不同立案星期下告知期限均为 20 个工作日之后。
func TestNoticeDueAcrossFilingWeekdays(t *testing.T) {
	cases := []struct {
		filed time.Time
		want  time.Time
	}{
		{day(2026, time.August, 17), day(2026, time.September, 14)}, // 周一
		{day(2026, time.August, 18), day(2026, time.September, 15)}, // 周二
		{day(2026, time.August, 21), day(2026, time.September, 18)}, // 周五
		{day(2026, time.August, 22), day(2026, time.September, 18)}, // 周六
		{day(2026, time.August, 23), day(2026, time.September, 18)}, // 周日
	}
	for _, tc := range cases {
		s, err := Compute("HJ-X", tc.filed)
		if err != nil {
			t.Fatalf("Compute 返回错误: %v", err)
		}
		if !s.NoticeDue.Equal(tc.want) {
			t.Errorf("立案 %s (%s): 告知期限 = %s, 期望 %s",
				tc.filed.Format("2006-01-02"), tc.filed.Weekday(),
				s.NoticeDue.Format("2006-01-02"), tc.want.Format("2006-01-02"))
		}
	}
}

// TestNoticeNotOverdueOnDueDate 断言在期限当日不算超期。
func TestNoticeNotOverdueOnDueDate(t *testing.T) {
	s, err := Compute("HJ-2026-001", day(2026, time.August, 17))
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	dueDate := day(2026, time.September, 14)
	if s.NoticeOverdue(dueDate) {
		t.Errorf("期限当日 %s 不应判定为超期", dueDate.Format("2006-01-02"))
	}
	if s.NoticeOverdue(day(2026, time.September, 11)) {
		t.Errorf("期限前 %s 不应判定为超期", "2026-09-11")
	}
	if !s.NoticeOverdue(day(2026, time.September, 15)) {
		t.Errorf("期限次日应判定为超期")
	}
}

func TestNegotiationDueFollowsNotice(t *testing.T) {
	s, err := Compute("HJ-2026-001", day(2026, time.August, 17))
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	want := day(2026, time.September, 14).AddDate(0, 0, NegotiationDays)
	if !s.NegotiationDue.Equal(want) {
		t.Fatalf("磋商期限 = %s, 期望 %s",
			s.NegotiationDue.Format("2006-01-02"), want.Format("2006-01-02"))
	}
}

func TestAssessmentAndLitigationDeadlines(t *testing.T) {
	filed := day(2026, time.August, 17)
	s, err := Compute("HJ-2026-001", filed)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if !s.AssessmentDue.Equal(filed.AddDate(0, 0, 60)) {
		t.Errorf("鉴定期限 = %s", s.AssessmentDue.Format("2006-01-02"))
	}
	if !s.LitigationBarredAt.Equal(filed.AddDate(3, 0, 0)) {
		t.Errorf("诉讼时效届满 = %s", s.LitigationBarredAt.Format("2006-01-02"))
	}
	if s.AssessmentOverdue(filed.AddDate(0, 0, 60)) {
		t.Errorf("鉴定期限当日不应超期")
	}
	if !s.AssessmentOverdue(filed.AddDate(0, 0, 61)) {
		t.Errorf("鉴定期限次日应超期")
	}
	if s.NegotiationOverdue(s.NegotiationDue) {
		t.Errorf("磋商期限当日不应届满")
	}
	if !s.LitigationBarred(filed.AddDate(3, 0, 1)) {
		t.Errorf("超过 3 年应判定时效届满")
	}
}

func TestCheckReturnsExpiredSentinel(t *testing.T) {
	filed := day(2026, time.August, 17)
	s, err := Compute("HJ-2026-001", filed)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if err := s.Check(filed.AddDate(1, 0, 0)); err != nil {
		t.Fatalf("时效内不应报错: %v", err)
	}
	if err := s.Check(filed.AddDate(3, 0, 1)); !errors.Is(err, model.ErrDeadlineExpired) {
		t.Fatalf("时效届满应返回 ErrDeadlineExpired, 实际 %v", err)
	}
}

func TestComputeRejectsZeroDate(t *testing.T) {
	if _, err := Compute("HJ-X", time.Time{}); !errors.Is(err, model.ErrInvalidClaim) {
		t.Fatalf("缺少立案日期应返回 ErrInvalidClaim, 实际 %v", err)
	}
}

// TestWorkdaysBetween 断言工作日区间统计不含起始日、含结束日。
func TestWorkdaysBetween(t *testing.T) {
	cases := []struct {
		from, to time.Time
		want     int
	}{
		{day(2026, time.August, 17), day(2026, time.August, 18), 1},
		{day(2026, time.August, 17), day(2026, time.August, 21), 4},
		{day(2026, time.August, 17), day(2026, time.August, 24), 5},
		{day(2026, time.August, 17), day(2026, time.August, 17), 0},
		{day(2026, time.August, 18), day(2026, time.August, 17), 0},
	}
	for _, tc := range cases {
		got := WorkdaysBetween(tc.from, tc.to)
		if got != tc.want {
			t.Errorf("WorkdaysBetween(%s, %s) = %d, 期望 %d",
				tc.from.Format("2006-01-02"), tc.to.Format("2006-01-02"), got, tc.want)
		}
	}
}

// TestRemainingWorkdaysForNotice 断言剩余工作日与告知期限一致。
func TestRemainingWorkdaysForNotice(t *testing.T) {
	filed := day(2026, time.August, 17)
	s, err := Compute("HJ-2026-001", filed)
	if err != nil {
		t.Fatalf("Compute 返回错误: %v", err)
	}
	if got := s.RemainingWorkdaysForNotice(filed); got != NoticeWorkdays {
		t.Fatalf("立案当日剩余工作日 = %d, 期望 %d", got, NoticeWorkdays)
	}
	if got := s.RemainingWorkdaysForNotice(s.NoticeDue); got != 0 {
		t.Fatalf("期限当日剩余工作日 = %d, 期望 0", got)
	}
}
