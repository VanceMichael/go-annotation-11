package claim

import (
	"fmt"
	"strings"

	"ecoclaim/internal/model"
)

// ValidationError 描述案件材料校验未通过的具体缺失项。
type ValidationError struct {
	ClaimID string
	Missing []string
}

// Error 返回缺失项的可读描述。
func (e *ValidationError) Error() string {
	return fmt.Sprintf("案件 %s 结案材料缺失: %s", e.ClaimID, strings.Join(e.Missing, "、"))
}

// Unwrap 使调用方可以通过 errors.Is 判定为材料校验失败。
func (e *ValidationError) Unwrap() error {
	return model.ErrValidationFailed
}

// Items 返回缺失项列表。
func (e *ValidationError) Items() []string {
	out := make([]string, len(e.Missing))
	copy(out, e.Missing)
	return out
}

// missingItems 收集案件的结案材料缺失项。
func missingItems(c model.Claim) []string {
	var missing []string
	if strings.TrimSpace(c.Title) == "" {
		missing = append(missing, "案由")
	}
	if strings.TrimSpace(c.RespondentID) == "" {
		missing = append(missing, "赔偿义务人")
	}
	if c.FiledAt.IsZero() {
		missing = append(missing, "立案日期")
	}
	if c.AffectedArea <= 0 && c.Kind != model.KindNoise {
		missing = append(missing, "受影响面积")
	}
	if c.AffectedDays <= 0 {
		missing = append(missing, "损害持续天数")
	}
	if c.BaselineCost <= 0 {
		missing = append(missing, "基准费用")
	}
	if c.AwardedAmount <= 0 {
		missing = append(missing, "核定赔偿金额")
	}
	if len(c.Transitions) == 0 {
		missing = append(missing, "阶段流转记录")
	}
	return missing
}

// Validate 校验案件的结案材料是否齐备。
//
// 材料齐备时返回 nil；存在缺失项时返回 *ValidationError，
// 调用方可通过 errors.Is 判定为 model.ErrValidationFailed。
func Validate(c model.Claim) error {
	var verr *ValidationError
	if missing := missingItems(c); len(missing) > 0 {
		verr = &ValidationError{ClaimID: c.ID, Missing: missing}
	}
	if verr == nil {
		return nil
	}
	return verr
}

// MissingCount 返回案件的结案材料缺失项数量。
func MissingCount(c model.Claim) int {
	return len(missingItems(c))
}
