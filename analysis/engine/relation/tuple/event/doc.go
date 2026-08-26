// Package event owns the signed tuple-event boundary between differential
// state reads and the evaluator.  Bind materializes one ordered vector of
// immutable before/after Tuple sides from an authenticated database Delta;
// events do not borrow state rows after Bind returns and do not mint an event
// identity or a second row representation.
//
// A missing side is sparse state.  In particular, a missing After is a signed
// deletion, while a present Tuple containing a ProvenAbsent cell remains a
// present side.  The distinction is retained by Event.Before/After rather
// than being collapsed into a default tuple.
package event
