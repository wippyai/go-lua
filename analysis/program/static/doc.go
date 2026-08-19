// Package static owns authored static syntax and sidecars attached to
// canonical Program Terms. It owns no inferred, domain, Link, or Target facts.
//
// # Shape
//
// Every typed vertical owns its own subpackage. A vertical owns its authored
// input vocabulary, its sealed row table, its query surface, and its section
// codec, and hands the sealed table back to this package as a value:
//
//	types         references    declarations  signatures
//	contracts     operators     operands      publications
//
// Their rows are built on the shared sealed-row substrate in
// analysis/program/internal/rows, and their decoders on the shared wire
// discipline in analysis/program/internal/wire. Reference direction between a
// vertical and this shell is enforced by the compiler, not by convention.
//
// No vertical reaches into another's storage. A law that spans two verticals
// consumes a column the owning vertical publishes -- Declarations publishes
// its interface-method pairs and its alias type-parameter claims, Signatures
// publishes a callable's scope and its binder-last formal rule, References
// publishes a reference's binder disposition, Types publishes a primitive row
// -- and the law itself lives here.
//
// # What this shell owns
//
//	the census        the one cardinality column, sealed once from authored
//	                  input and thereafter the sole cardinality authority
//	the constructor   Build, a pure staged seal: census, then the independent
//	                  verticals, then the verticals that consume a sealed
//	                  sibling, then the joint laws, then the content identity
//	the joint laws    the combined containment forest, exactly-once TypeParam
//	                  ownership, the interface-method scope join, and the
//	                  bound-assertion direct-return rule
//	the lifecycle     Draft, Finalizer, View: one publication transaction,
//	                  whose fence every owner-view accessor checks exactly once
//	the stream        section order and record framing for the artifact
//	                  payload, shared with the ContentID digest
//
// # Altitude law
//
// The shell itself still spans five altitudes in one package, ordered
//
//	core < read < codec < identity < build
//
// and references flow one way only. The compiler cannot enforce direction
// inside a package, so TestAltitudeLawIsOneWay does. Altitude is read off the
// file name:
//
//	core      doc.go, model.go, *_model.go, families.go, api.go
//	read      query*.go, counts.go, lifecycle_view.go
//	codec     artifact_section_*.go
//	identity  content.go, identity.go
//	build     every other file
package static
