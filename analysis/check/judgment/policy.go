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
