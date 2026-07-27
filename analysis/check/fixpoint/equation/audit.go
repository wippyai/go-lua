package equation

// AccessRecord is the dynamic audit payload recorded by an existing bound
// kernel.  The compiler never inspects it to influence evaluation.
type AccessRecord struct {
	Reads, Writes, Advances, Outcomes, Diagnostics, Dependencies []string
	// Payload retains the source owner's exact audit record.  It is opaque to
	// the equation package so audit plumbing cannot reinterpret a contract or
	// grow an alternate access vocabulary.
	Payload any
}
