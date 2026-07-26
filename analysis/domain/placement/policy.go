package placement

import "github.com/wippyai/go-lua/analysis/domain/placement/vocab"

// EscapeTransition is a compatibility alias for the canonical escape
// vocabulary. Call-boundary events are translated into this neutral vocabulary
// by fact application.
type EscapeTransition = vocab.Escape

const (
	EscapeTransitionNone   = vocab.None
	EscapeTransitionReturn = vocab.Return
	EscapeTransitionRetain = vocab.Retain
	EscapeTransitionStore  = vocab.Store
	EscapeTransitionSend   = vocab.Send
	EscapeTransitionExport = vocab.Export
	EscapeTransitionOpaque = vocab.Opaque
)
