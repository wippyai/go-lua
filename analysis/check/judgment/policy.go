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

var defaultPolicy = NewPolicy(defaultPolicyLevels())

func defaultPolicyLevels() map[PolicyKey]Level {
	levels := make(map[PolicyKey]Level, len(defaultRegistry.Codes())*9)
	addStrictnessTunableTypeErrors(levels,
		CodeCallArgType,
		CodeCallArity,
		CodeCallCallee,
		CodeAssignment,
		CodeAssignmentTarget,
		CodeReturn,
	)
	addRefutedErrors(levels,
		CodeNonNilAssertion,
		CodeNumericForOperand,
		CodeUnresolvedValue,
		CodeUnresolvedType,
		CodeMemberRead,
	)
	addRefutedWarnings(levels,
		CodeFrozenTable,
		CodeLifecycle,
		CodeUnusedLocal,
		CodeDeadAssignment,
		CodeChannelSelect,
		CodeDiscriminatedUnion,
		CodeOptional,
		CodeResultShape,
		CodeRegistration,
		CodeTableDispatch,
		CodeRedundantCondition,
		CodeConcatOperand,
	)
	addHints(levels, CodeSendIsolation)
	addProvenHints(levels,
		CodeAdviceRedundantClaim,
		CodeAdviceAlwaysTrueGuard,
		CodeAdviceInvariantLoopRead,
	)
	return levels
}

func addStrictnessTunableTypeErrors(levels map[PolicyKey]Level, codes ...Code) {
	for _, code := range codes {
		addPolicyRows(levels, code, LevelError, LevelError, LevelWarning, LevelError, LevelError, LevelError)
	}
}

func addRefutedErrors(levels map[PolicyKey]Level, codes ...Code) {
	for _, code := range codes {
		addPolicyRows(levels, code, LevelDisabled, LevelError, LevelDisabled, LevelError, LevelDisabled, LevelError)
	}
}

func addRefutedWarnings(levels map[PolicyKey]Level, codes ...Code) {
	for _, code := range codes {
		addPolicyRows(levels, code, LevelDisabled, LevelWarning, LevelDisabled, LevelWarning, LevelDisabled, LevelWarning)
	}
}

func addHints(levels map[PolicyKey]Level, codes ...Code) {
	for _, code := range codes {
		for _, mode := range []StrictnessMode{StrictnessDefault, StrictnessLenient, StrictnessStrict} {
			levels[PolicyKey{Code: code, Verdict: VerdictProven, Mode: mode}] = LevelHint
			levels[PolicyKey{Code: code, Verdict: VerdictUnknown, Mode: mode}] = LevelHint
			levels[PolicyKey{Code: code, Verdict: VerdictRefuted, Mode: mode}] = LevelHint
		}
	}
}

func addProvenHints(levels map[PolicyKey]Level, codes ...Code) {
	for _, code := range codes {
		addProvenHintRows(levels, code, StrictnessDefault)
		addProvenHintRows(levels, code, StrictnessLenient)
		addProvenHintRows(levels, code, StrictnessStrict)
	}
}

func addProvenHintRows(levels map[PolicyKey]Level, code Code, mode StrictnessMode) {
	levels[PolicyKey{Code: code, Verdict: VerdictProven, Mode: mode}] = LevelHint
	levels[PolicyKey{Code: code, Verdict: VerdictUnknown, Mode: mode}] = LevelDisabled
	levels[PolicyKey{Code: code, Verdict: VerdictRefuted, Mode: mode}] = LevelDisabled
}

func addPolicyRows(
	levels map[PolicyKey]Level,
	code Code,
	defaultUnknown Level,
	defaultRefuted Level,
	lenientUnknown Level,
	lenientRefuted Level,
	strictUnknown Level,
	strictRefuted Level,
) {
	addPolicyModeRows(levels, code, StrictnessDefault, defaultUnknown, defaultRefuted)
	addPolicyModeRows(levels, code, StrictnessLenient, lenientUnknown, lenientRefuted)
	addPolicyModeRows(levels, code, StrictnessStrict, strictUnknown, strictRefuted)
}

func addPolicyModeRows(levels map[PolicyKey]Level, code Code, mode StrictnessMode, unknown Level, refuted Level) {
	levels[PolicyKey{Code: code, Verdict: VerdictProven, Mode: mode}] = LevelDisabled
	levels[PolicyKey{Code: code, Verdict: VerdictUnknown, Mode: mode}] = unknown
	levels[PolicyKey{Code: code, Verdict: VerdictRefuted, Mode: mode}] = refuted
}

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
