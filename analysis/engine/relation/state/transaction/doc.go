// Package transaction validates proposals and prepares atomic monotone
// ascents. NewSubmissionBatch admits one application;
// NewDifferentialSubmissionBatch admits one signed before/after Differential
// while keeping ordinary writes on After and contribution transitions on the
// exact retained sides. The parent state/database package owns the sole root
// Commit and returns the canonical relation deltas consumed by the solver.
package transaction
