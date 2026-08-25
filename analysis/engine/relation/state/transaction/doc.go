// Package transaction validates proposals and prepares atomic monotone
// ascents. The parent state/database package owns the sole root Commit and
// returns the canonical relation deltas consumed by the solver.
package transaction
