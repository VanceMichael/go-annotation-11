package model

import "errors"

// 领域哨兵错误。上层通过 errors.Is 判定错误类别，禁止依赖错误文本。
var (
	// ErrInvalidClaim 案件登记信息非法。
	ErrInvalidClaim = errors.New("model: 案件登记信息非法")
	// ErrUnknownDamageKind 损害类型代码不存在。
	ErrUnknownDamageKind = errors.New("model: 未知损害类型")
	// ErrUnknownSeverity 损害程度代码不存在。
	ErrUnknownSeverity = errors.New("model: 未知损害程度")
	// ErrUnknownStage 阶段代码不存在。
	ErrUnknownStage = errors.New("model: 未知案件阶段")
	// ErrClaimUnknown 案件不存在。
	ErrClaimUnknown = errors.New("model: 案件不存在")
	// ErrRespondentUnknown 赔偿义务人不存在。
	ErrRespondentUnknown = errors.New("model: 赔偿义务人不存在")
	// ErrDuplicateClaim 案件重复登记。
	ErrDuplicateClaim = errors.New("model: 案件重复登记")
	// ErrStageConflict 阶段流转非法。
	ErrStageConflict = errors.New("model: 案件阶段流转非法")
	// ErrDeadlineExpired 法定时限已届满。
	ErrDeadlineExpired = errors.New("model: 法定时限已届满")
	// ErrAssessmentIncomplete 损害鉴定评估未完成。
	ErrAssessmentIncomplete = errors.New("model: 损害鉴定评估未完成")
	// ErrValidationFailed 案件材料校验未通过。
	ErrValidationFailed = errors.New("model: 案件材料校验未通过")
	// ErrGatewayUnavailable 外部鉴定评估网关不可用。
	ErrGatewayUnavailable = errors.New("model: 鉴定评估网关不可用")
)
