package summary

// ParamCondition is a summary-local truthiness condition for one parameter on
// normal return.
type ParamCondition uint8

const (
	ParamConditionBottom ParamCondition = iota
	ParamConditionTruthy
	ParamConditionFalsy
	ParamConditionTop
)

// IsUseful reports whether c carries a caller-applicable condition.
func (c ParamCondition) IsUseful() bool {
	return c == ParamConditionTruthy || c == ParamConditionFalsy
}

func paramConditionLessOrEq(a, b ParamCondition) bool {
	return a == b || a == ParamConditionBottom || b == ParamConditionTop
}

func joinParamCondition(a, b ParamCondition) ParamCondition {
	if a == b {
		return a
	}
	if a == ParamConditionBottom {
		return b
	}
	if b == ParamConditionBottom {
		return a
	}
	return ParamConditionTop
}

func widenParamCondition(prev, next ParamCondition) ParamCondition {
	if prev == next {
		return next
	}
	if prev == ParamConditionBottom {
		return next
	}
	return ParamConditionTop
}
