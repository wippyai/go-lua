// Package dominance provides dominator, dominance frontier, and post-dominance
// utilities as reusable analysis surfaces.
//
// These helpers are intentionally kept as theory leaves so higher layers can
// wire them into CFG and SSA consumers without changing the package shape.
package dominance
