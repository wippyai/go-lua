// Package execution owns the small, engine-private execution substrate used
// by typed generated lanes.  It carries lifecycle authority only; semantic
// values remain in the typed factbinding plane.
//
// The disposition of an execution attempt is structure.ReductionOutcome: the
// one five-valued outcome vocabulary the analyzer declares, shared with the
// folds that produce it, the activation relation, and the Delta path that
// consumes it.  This package holds no outcome enum of its own, so a lane
// cannot conclude in a disposition its consumers have no member for.
package execution
