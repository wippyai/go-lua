package placement

// EscapeTransition is a compatibility alias for the canonical escape
// vocabulary. Call-boundary events are translated into this neutral vocabulary
// by fact application.
type EscapeTransition = Escape

const (
	EscapeTransitionNone   = None
	EscapeTransitionReturn = Return
	EscapeTransitionRetain = Retain
	EscapeTransitionStore  = Store
	EscapeTransitionSend   = Send
	EscapeTransitionExport = Export
	EscapeTransitionOpaque = Opaque
)
