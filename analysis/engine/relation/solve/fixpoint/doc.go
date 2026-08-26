// Package fixpoint owns the solve-local semi-naive work queue. It consumes the
// dependency/SCC schedule sealed inside arrangement.Execution and immutable
// database roots/deltas; it does not own a second graph, a relation store, an
// evaluator callback, or domain vocabulary.
//
// Full roots seed every owner-issued dependency in sealed component order.
// Later roots carry one authenticated database.Delta and wake only the
// execution-owned relation-wide/exact-column projections. The queue never
// rescans arrangement bindings, resolves an Access, or carries a physical
// node identity in Work.
//
// ScheduleEntry retains exact recurrence-head relation IDs. Evaluators and
// publication redeem Mounted.Widening(dependency, destinationRelation) only
// after checking that entry's WideningFor relation authorization. The queue
// itself carries no widening permit and cannot infer or fall back to one.
package fixpoint
