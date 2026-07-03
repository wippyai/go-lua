package judgment

// StrictnessMode selects how unknown obligations are surfaced. It is separate
// from judgment production so semantic facts stay stable while users tune
// enforcement.
type StrictnessMode string

const (
	StrictnessDefault StrictnessMode = "default"
	StrictnessLenient StrictnessMode = "lenient"
	StrictnessStrict  StrictnessMode = "strict"
)

// Level is the policy-layer disposition for a judgment. Renderers map this to
// user-facing diagnostic severity or suppression.
type Level uint8

const (
	LevelDisabled Level = iota
	LevelError
	LevelWarning
	LevelHint
)

type PolicyKey struct {
	Code    Code
	Verdict Verdict
	Mode    StrictnessMode
}

// Policy is the central judgment disposition table.
type Policy struct {
	levels map[PolicyKey]Level
}

var defaultPolicy = NewPolicy(map[PolicyKey]Level{
	{Code: CodeCallArgType, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeCallArgType, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallArgType, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallArgType, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeCallArgType, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeCallArgType, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeCallArgType, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeCallArgType, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeCallArgType, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeCallArity, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeCallArity, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallArity, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallArity, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeCallArity, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeCallArity, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeCallArity, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeCallArity, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeCallArity, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeCallCallee, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeCallCallee, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallCallee, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeCallCallee, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeCallCallee, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeCallCallee, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeCallCallee, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeCallCallee, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeCallCallee, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeAssignment, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeAssignment, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeAssignment, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeAssignment, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeAssignment, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeAssignment, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeAssignment, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeAssignment, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeAssignment, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeAssignmentTarget, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeAssignmentTarget, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeAssignmentTarget, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeAssignmentTarget, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeAssignmentTarget, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeAssignmentTarget, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeAssignmentTarget, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeAssignmentTarget, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeAssignmentTarget, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeReturn, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeReturn, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelError,
	{Code: CodeReturn, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeReturn, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeReturn, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeReturn, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeReturn, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeReturn, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelError,
	{Code: CodeReturn, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeNonNilAssertion, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeNonNilAssertion, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeNonNilAssertion, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeNonNilAssertion, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeNumericForOperand, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeNumericForOperand, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeNumericForOperand, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeNumericForOperand, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeFrozenTable, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeFrozenTable, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeFrozenTable, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeFrozenTable, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeLifecycle, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeLifecycle, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeLifecycle, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeLifecycle, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeUnusedLocal, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeUnusedLocal, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeUnusedLocal, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeUnusedLocal, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeDeadAssignment, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeDeadAssignment, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeDeadAssignment, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeDeadAssignment, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeChannelSelect, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeChannelSelect, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeChannelSelect, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeChannelSelect, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeDiscriminatedUnion, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeDiscriminatedUnion, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeResultShape, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeResultShape, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeResultShape, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeResultShape, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeRegistration, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeRegistration, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeRegistration, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeRegistration, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeUnresolvedValue, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeUnresolvedValue, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeUnresolvedValue, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeUnresolvedValue, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeUnresolvedType, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeUnresolvedType, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeUnresolvedType, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeUnresolvedType, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeRedundantCondition, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeRedundantCondition, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeRedundantCondition, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeRedundantCondition, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,

	{Code: CodeMemberRead, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelError,
	{Code: CodeMemberRead, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelError,
	{Code: CodeMemberRead, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeMemberRead, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelError,

	{Code: CodeConcatOperand, Verdict: VerdictProven, Mode: StrictnessDefault}:  LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictUnknown, Mode: StrictnessDefault}: LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictRefuted, Mode: StrictnessDefault}: LevelWarning,
	{Code: CodeConcatOperand, Verdict: VerdictProven, Mode: StrictnessLenient}:  LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictUnknown, Mode: StrictnessLenient}: LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictRefuted, Mode: StrictnessLenient}: LevelWarning,
	{Code: CodeConcatOperand, Verdict: VerdictProven, Mode: StrictnessStrict}:   LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictUnknown, Mode: StrictnessStrict}:  LevelDisabled,
	{Code: CodeConcatOperand, Verdict: VerdictRefuted, Mode: StrictnessStrict}:  LevelWarning,
})

// DefaultPolicy returns the standard judgment disposition table.
func DefaultPolicy() Policy {
	return defaultPolicy
}

// IsZero reports whether p has no configured dispositions. It lets callers
// accept the zero-value policy and substitute DefaultPolicy at the boundary.
func (p Policy) IsZero() bool {
	return p.levels == nil
}

// NewPolicy builds a policy table.
func NewPolicy(levels map[PolicyKey]Level) Policy {
	out := make(map[PolicyKey]Level, len(levels))
	for key, level := range levels {
		if key.Mode == "" {
			key.Mode = StrictnessDefault
		}
		out[key] = level
	}
	return Policy{levels: out}
}

// LevelFor returns the configured disposition for a judgment in mode.
func (p Policy) LevelFor(j Judgment, mode StrictnessMode) (Level, bool) {
	if mode == "" {
		mode = StrictnessDefault
	}
	level, ok := p.levels[PolicyKey{Code: j.Code, Verdict: j.Verdict, Mode: mode}]
	return level, ok
}
