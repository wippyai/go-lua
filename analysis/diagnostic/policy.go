package diagnostic

// Policy applies caller-controlled per-code diagnostic enablement and severity
// overrides after diagnostics have been produced.
type Policy struct {
	Rules map[Code]Rule
}

// Rule configures one diagnostic code.
type Rule struct {
	Disabled    bool
	Enabled     bool
	HasEnabled  bool
	Severity    Severity
	HasSeverity bool
}

// Enable allows an opt-in diagnostic code to be produced.
func Enable() Rule {
	return Rule{Enabled: true, HasEnabled: true}
}

// Disable suppresses diagnostics for a code.
func Disable() Rule {
	return Rule{Disabled: true, HasEnabled: true}
}

// OverrideSeverity remaps diagnostics for a code to the provided severity.
func OverrideSeverity(severity Severity) Rule {
	return Rule{Severity: severity, HasSeverity: true}
}

// WithSeverity returns a copy of the rule with a severity override applied.
func (r Rule) WithSeverity(severity Severity) Rule {
	r.Severity = severity
	r.HasSeverity = true
	return r
}

// Apply filters and remaps diagnostics according to the policy.
func (p Policy) Apply(diags []Diagnostic) []Diagnostic {
	if len(diags) == 0 {
		return nil
	}
	out := make([]Diagnostic, 0, len(diags))
	for _, diag := range diags {
		diag, ok := p.ApplyOne(diag)
		if !ok {
			continue
		}
		out = append(out, diag)
	}
	return out
}

// ApplyOne filters or remaps one diagnostic according to the policy.
func (p Policy) ApplyOne(diag Diagnostic) (Diagnostic, bool) {
	if !p.Enabled(diag.Code, true) {
		return Diagnostic{}, false
	}
	rule := p.Rules[diag.Code]
	if rule.HasSeverity {
		diag.Severity = rule.Severity
	}
	return diag, true
}

// Enabled reports whether code should be produced when the producer's default
// enablement is defaultEnabled.
func (p Policy) Enabled(code Code, defaultEnabled bool) bool {
	rule, ok := p.Rules[code]
	if !ok || !rule.HasEnabled {
		return defaultEnabled
	}
	if rule.Disabled {
		return false
	}
	return rule.Enabled
}
