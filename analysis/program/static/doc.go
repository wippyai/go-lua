// Package static owns authored static syntax and sidecars attached to
// canonical Program Terms, including authored type-reference resolution. It
// owns no inferred, domain, Link, or Target facts. The Types vertical itself
// deliberately owns no TypeRef rows; References owns that exact relation.
//
// # Altitude law
//
// Every file in this package sits at exactly one of five altitudes, ordered
//
//	core < read < codec < identity < build
//
// and references flow one way only: a file may name a symbol defined at its
// own altitude or below it, never above. Core therefore references nothing in
// this package outside core, and no codec file may reach into a build file.
// The law is mechanically enforced by TestAltitudeLawIsOneWay.
//
// Altitude is read off the file name, so a new file declares its altitude by
// what it is called:
//
//	core      doc.go, model.go, *_model.go, families.go, api.go
//	read      query*.go, counts.go, lifecycle_view.go
//	codec     artifact_section_*.go
//	identity  content.go, identity.go
//	build     every other file
//
// The altitudes carry distinct authority. Core owns the authored row shapes,
// the stores that hold them, and the closed family inventories; it validates
// nothing and encodes nothing. Read owns the immutable query surfaces over a
// sealed Component. Codec owns the one wire schema, shared by the artifact
// payload and the ContentID digest so a single writer defines both. Identity
// seals the component. Build owns admission: it validates authored input,
// compacts it into the stores, and publishes.
//
// # Family matrix
//
// Eight typed verticals each occupy their own cell at core, read, codec, and
// build. Identity is whole-component and has no per-family cell.
//
//	family        core                    read                     codec                             build
//	------------  ----------------------  -----------------------  --------------------------------  ----------------
//	types         types_model.go          query.go                 artifact_section_types.go         types.go
//	references    references_model.go     query.go                 artifact_section_references.go    references.go
//	declarations  declarations_model.go   query_declared_types.go  artifact_section_declarations.go  declarations.go
//	signatures    signatures_model.go     query_signatures.go      artifact_section_signatures.go    signatures.go
//	contracts     contracts_model.go      query_contracts.go       artifact_section_contracts.go     contracts.go
//	operators     operators_model.go      query_operators.go       artifact_section_operators.go     operators.go
//	operands      operands_model.go       query_operands.go        artifact_section_operands.go      operands.go
//	publications  publications_model.go   query_publications.go    artifact_section_publications.go  publications.go
//
// query.go additionally holds the shared cursor vocabulary the types,
// references, and declarations cells read through.
//
// The files that serve every family rather than one of them are:
//
//	core      model.go (Component, Draft, Finalizer, View, poolRange),
//	          families.go (closed family inventories and census),
//	          static_type_model.go, containment_model.go, api.go
//	read      query_static_types.go, query_local_containment.go,
//	          lifecycle_view.go, counts.go
//	codec     artifact_section_codec.go (record framing and shared writers),
//	          artifact_section_decoder.go
//	identity  content.go, identity.go
//	build     build.go, validate.go
package static
