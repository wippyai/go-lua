// Package calleffect owns call-effect projection over transfer evidence.
//
// It reduces call evidence plus callee contracts into abstract-interpreter
// effects such as table mutations, container mutations, callback invocation, and
// captured nested-function mutation replay. Callers provide the solved type
// query they want to use; this package owns how call evidence is interpreted as
// effect payloads.
package calleffect
