// doc.go is the package file map: it names the ten planes of the engine, the one
// shared leaf vocabulary, the subpackages, and the files that belong to each. A
// file's plane is the plane its measured couplings point at: the map states
// where a file's declarations are consumed and which plane's declarations it
// consumes, not where its name sorts. The package doc itself lives with the
// semantic key declaration.
//
// # Declaration plane
//
// The callback-free cold surface a domain declares against. Nothing here holds
// an implementation, a runtime slot, or a solve-local value.
//
//	composition.go        sealed Schema identity and the Composition it seals
//	schema_slots.go       the reusable cold slot surface and its shape queries
//	factor.go             Factor measure, algebra and transition witness
//	rule.go               typed positional Read and Rule capabilities
//	rule_operand.go       the synthetic operand engine laws declare against
//	operand_entity.go     the opaque equation-owned content identity
//	framed_identity.go    canonical framed preimages for engine identities
//	semantic_key.go       the one-way conversion to the cold canonical key
//	generation_cell.go    the lock-free live-stamp cell
//
// # Binding plane
//
// The schema binding transaction: the cells a Link fills, the seal that freezes
// them, and the Layer-B tokens the seal issues.
//
//	schema_binding.go            transaction lifecycle: state, phase, seal, poison
//	schema_bind_entry.go         the public Bind entry points and implementation accessors
//	schema_rule_binding.go       Rule hot specs, carry, read origin, selected-route transaction
//	schema_rule_read_binding.go  the typed and opaque Rule read binding implementations
//	schema_factor_binding.go     the Factor, form and Rule binding cells
//	schema_seal_tokens.go        the tokens a sealed binding issues and the fences validating them
//	schema_activation_binding.go the Link-local activation implementation half
//	schema_query_binding.go      the query binding cells and sealed implementations
//	schema_query_runtime.go      the common solver query implementation
//	rule_capability.go           declaration-time Rule slot capability machinery
//	rule_surface.go              binding-surface value constructors
//
// # Runtime assembly plane
//
// The one pass that lowers a sealed binding plus a topology into the immutable
// solver runtime.
//
//	runtime_binding.go         read form kinds, the runtimeBinding container, catalog freeze
//	runtime_binding_catalog.go graph use rows, carry closures, and binding catalog
//	runtime_factor_bind.go     the per-Factor binding pass and its shape matchers
//	runtime_factor.go          the bound Factor vocabulary and its runtime methods
//	runtime_assembly.go        the solver runtime vocabulary and the assembly pass
//	runtime_member.go          the Rule and activation members and their geometry
//	runtime_regions.go         static dependency edges, region binding, demand membership
//	runtime_reindex.go         the immutable lowering of every equation reindex
//
// # Solve loop plane
//
// The region fixpoint: one epoch per Solve, driven point by point.
//
//	runtime_executor.go         the region fixpoint driver and the public Solve entries
//	runtime_epoch.go            epoch vocabulary, point queue, failure recorders, lifecycle
//	runtime_epoch_activation.go accepted-activation canonicalization and overlay install
//	runtime_epoch_queue.go      dirty and structural marking, postfix proofs, enqueue and take
//	runtime_point_fold.go       producer inputs, Rule evaluation, fold term algebra
//	runtime_point_refresh.go    point publication and refresh
//	runtime_region_interface.go region right-hand sides, interface refresh, restart, settlement
//	runtime_selected_overlay.go the stale-fenced structural overlay
//	runtime_diagnostics.go      the solve-local aggregate diagnostics
//
// # Execution frame plane
//
// The row-local surface a Rule body sees while it runs, and nothing else.
//
//	runtime_frame.go  the opaque Fold frame/result API and private product session
//	runtime_read.go   typed and staged read runtimes, selection sessions, materialization
//	runtime_output.go output access, typed staging, carry transform, patch publication
//	runtime_rule.go   the bound Rule and engine-owned Fold execution
//	selector.go       the row-local SelectorContext capability
//
// # Sealed Rule geometry
//
// SchemaBinding finalizes each ordinary Rule cell once. Runtime consumes that
// cell's direct geometry and immutable read rows; it carries no callback
// admission, derivation replay, ticket, proof, or evidence plane.
//
// # Activation plane
//
// Activation results and the candidates that produce them.
//
//	activation.go                    pure activation Fold/result and topology settlement
//	activation_candidate_binding.go  the mounted candidate issuer binding
//
// # Query and observation plane
//
// The published read surface over a solved State.
//
//	query.go               the typed persistence contract for one frozen Query
//	query_result.go        one published transitively immutable result value
//	runtime_observation.go the solver-side observation runtime
//	runtime_provenance.go  the exact observation identities of one live product
//
// # Publication plane
//
// The one write door into a published snapshot column. A column is filled by
// the capability the engine minted for the writer its sealed table admitted,
// and the published value carries none.
//
//	publication_column.go   admitted (column, writer) pairs, the minted capability, the write verbs
//	snapshot_materialize.go one completed solve published as immutable snapshot columns
//
// # State and results plane
//
//	state.go one completed immutable Solver result
//
// # Failure vocabulary
//
// One leaf shared by the solve loop and activation: the
// reason a solve refused, the boundary that named it, and the published report.
// It belongs to no plane because it declares nothing about any plane's
// machinery. Advisory tiers are config-gated, never removed.
//
//	solve_report.go the failure reason, the solve boundary, the program
//	                construction stages and the solve report
//
// # Compile plane
//
// Construction consumes the schema/compose-owned sealed ProgramRule rows at
// the direct root and folds them into one immutable committed program. Solved
// results are read from Snapshot.Query by family and stable row identity.
//
// # Subpackages
//
//	rows                        the public row vocabulary shared with domains
//	internal/composition        cold canonical identity and shape rows
//	internal/equation           the point, group and member topology
//	internal/carrier            point state, contributions and change sets
//	internal/carrier/product    product arity and slot geometry
//	internal/carrier/shape      carrier shape identity
//	internal/demand             observations and demand membership
//	internal/factbinding        fact binding for domain algebras
//	internal/facts              typed fact families: diagram, scalar, semantic, stage, support, terminal
//	internal/frame              frame-local row storage
//	internal/guard              guard sets and their intersection
//	internal/schedule           the execution schedule, its events and regions

package engine
