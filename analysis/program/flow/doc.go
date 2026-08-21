// Package flow owns the authored Flow relation and the one published Flow
// component. It owns no second query authority over its child results.
//
// # Shape
//
// Every Flow judgment lives in its own public sibling package. A sibling owns
// its authored input, its sealed rows, its query surface, and its proofs, and
// hands the sealed result back to this shell as a value:
//
//	authored      body          binding       containment
//	control       position      semanticpath  sourcecontrol
//	outcome       executable    evaluation    continuation
//	candidates    directfunction accessgeometry binaryprimitive
//	functionboundary returnprojection runtimeentry routeplan
//	recurrence    causal        staticcheck
//	kind role provenance        shared vocabulary
//
// Reference direction is under the compiler, not under review: a sibling never
// names this shell, this shell names the siblings it assembles, and a
// downstream consumer names the sibling that owns the row it reads. The law in
// reference_direction_law_test.go states that direction and keeps it honest.
//
// # What this shell owns
//
//	the seal path     Assemble consumes the authored Draft with its Source,
//	                  Static, and Module siblings and publishes one Component
//	                  and its View. View validates the composition fence, then
//	                  returns the sealed result owned by the child package;
//	                  it does not redeclare or copy that result's vocabulary.
//	the denominator   CountRows over sealed owner projections.
//	the identities    the allocation-path, value-source, storage, call, and
//	                  function content identities minted once at assembly.
package flow
