// Package architecture compiles one reviewed containment ownership boundary
// into the mechanically complete cutplan.Intent needed by flashrefactor.
//
// The caller declares only the semantic decision: the old/new authority,
// exact source fields, the exact destination identity and containment edge,
// and exact bounded laws. CollectSurvey obtains source evidence from the sole
// Go resolver. Compile then derives relocation subjects, supported consumer
// routes, necessary target import, exact read/write footprint, and mandatory
// structural gates.
//
// This is deliberately not a generic refactoring DSL. Its only accepted cut
// is direct named-field containment extraction. A promoted field, external
// keyed literal, existing target child, unresolved source object, or any other
// use that cannot be routed by the finite renderer rejects the whole cut.
// Survey has no Lock or apply conversion; Compile returns a reviewed Intent
// which still has to pass through the normal write-free Prepare -> Lock path.
package architecture
