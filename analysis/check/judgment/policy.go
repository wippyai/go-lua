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

// PolicyConfig is the user-tunable judgment severity surface. The zero value
// uses DefaultPolicy in StrictnessDefault mode.
type PolicyConfig struct {
	Policy     Policy
	Strictness StrictnessMode
}

var defaultPolicy = NewPolicy(defaultPolicyLevels())

type policyModeLevels struct {
	Proven  Level
	Unknown Level
	Refuted Level
}

type policyProfileLevels struct {
	Default policyModeLevels
	Lenient policyModeLevels
	Strict  policyModeLevels
}

var defaultPolicyProfiles = map[PolicyProfile]policyProfileLevels{
	PolicyStrictnessTunableTypeError: {
		Default: policyModeLevels{Proven: LevelDisabled, Unknown: LevelError, Refuted: LevelError},
		Lenient: policyModeLevels{Proven: LevelDisabled, Unknown: LevelWarning, Refuted: LevelError},
		Strict:  policyModeLevels{Proven: LevelDisabled, Unknown: LevelError, Refuted: LevelError},
	},
	PolicyRefutedError: {
		Default: policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelError},
		Lenient: policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelError},
		Strict:  policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelError},
	},
	PolicyRefutedWarning: {
		Default: policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelWarning},
		Lenient: policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelWarning},
		Strict:  policyModeLevels{Proven: LevelDisabled, Unknown: LevelDisabled, Refuted: LevelWarning},
	},
	PolicyHint: {
		Default: policyModeLevels{Proven: LevelHint, Unknown: LevelHint, Refuted: LevelHint},
		Lenient: policyModeLevels{Proven: LevelHint, Unknown: LevelHint, Refuted: LevelHint},
		Strict:  policyModeLevels{Proven: LevelHint, Unknown: LevelHint, Refuted: LevelHint},
	},
	PolicyProvenHint: {
		Default: policyModeLevels{Proven: LevelHint, Unknown: LevelDisabled, Refuted: LevelDisabled},
		Lenient: policyModeLevels{Proven: LevelHint, Unknown: LevelDisabled, Refuted: LevelDisabled},
		Strict:  policyModeLevels{Proven: LevelHint, Unknown: LevelDisabled, Refuted: LevelDisabled},
	},
}

func defaultPolicyLevels() map[PolicyKey]Level {
	levels := make(map[PolicyKey]Level, len(defaultRegistry.Codes())*9)
	for _, code := range defaultRegistry.Codes() {
		spec, ok := defaultRegistry.Lookup(code)
		if !ok {
			panic("judgment: default registry code disappeared")
		}
		rows, ok := defaultPolicyProfiles[spec.Policy]
		if !ok {
			panic("judgment: missing default policy profile for " + string(code))
		}
		addPolicyModeRows(levels, code, StrictnessDefault, rows.Default)
		addPolicyModeRows(levels, code, StrictnessLenient, rows.Lenient)
		addPolicyModeRows(levels, code, StrictnessStrict, rows.Strict)
	}
	return levels
}

func addPolicyModeRows(levels map[PolicyKey]Level, code Code, mode StrictnessMode, rows policyModeLevels) {
	levels[PolicyKey{Code: code, Verdict: VerdictProven, Mode: mode}] = rows.Proven
	levels[PolicyKey{Code: code, Verdict: VerdictUnknown, Mode: mode}] = rows.Unknown
	levels[PolicyKey{Code: code, Verdict: VerdictRefuted, Mode: mode}] = rows.Refuted
}

// DefaultPolicy returns the standard judgment disposition table.
func DefaultPolicy() Policy {
	return defaultPolicy
}

// Normalized returns c with zero-value fields replaced by judgment defaults.
func (c PolicyConfig) Normalized() PolicyConfig {
	if c.Policy.IsZero() {
		c.Policy = DefaultPolicy()
	}
	if c.Strictness == "" {
		c.Strictness = StrictnessDefault
	}
	return c
}

// LevelFor returns the configured disposition for a judgment.
func (c PolicyConfig) LevelFor(j Judgment) (Level, bool) {
	c = c.Normalized()
	return c.Policy.LevelFor(j, c.Strictness)
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
