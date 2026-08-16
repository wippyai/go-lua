// Package executable seals Flow's pre-Outcome runtime membership.
//
// The result is a Source/Flow/Static/Module-identity-bound, dense per-family bitset. Source
// control supplies the reachable direct roots; containment supplies static
// classification and the complete pre-Outcome denominator. Authored Flow
// operands are then closed iteratively. No source position table, causal
// edge, Outcome, or consumer-specific projection is retained here. The
// containment and source-control owners retain only scalar owner value fences
// and expose narrow Matches checks; there is no pointer authority
// or generic assembly token to splice. This package validates its complete
// denominator and self-membership before closing the runtime operands.
package executable
