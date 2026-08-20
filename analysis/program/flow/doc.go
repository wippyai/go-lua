// Package flow owns the authored Flow relation and the one published Flow
// component. It owns no query authority of its own beyond the two capability
// fences named below.
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
//	the seal path     Build takes the authored vocabulary into a construction
//	                  Draft; Assemble consumes that Draft with its Source,
//	                  Static, and Module siblings and publishes one Component
//	                  and its View. View routes a query to its owner and
//	                  applies the composition fence before handing the owner
//	                  out.
//	the fences        Draft withholds the authored Finalizer, Authored
//	                  withholds the authored owner's internal storage, Outcomes withholds the
//	                  owner's Find join, and Ports withholds the sealed term
//	                  denominator. Each is a capability this altitude declines
//	                  to publish, not a copy of an owner's rows.
//	the codec         WriteArtifactSection, ReadArtifactSection, and CountRows
//	                  on the authored payload.
//	the identities    the allocation-path, value-source, storage, call, and
//	                  function content identities minted once at assembly.
package flow
