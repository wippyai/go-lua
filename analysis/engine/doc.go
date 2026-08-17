// doc.go is the package file map: it names the twelve planes of the engine and
// the files that belong to each one. The package doc itself lives with the
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
//	rule_surface.go       binding-surface value constructors
//	rule_capability.go    declaration-time Rule slot capability machinery
//	selector.go           the row-local SelectorContext capability
//	exact_ref.go          the private factor-coordinate projection
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
//
// # Runtime assembly plane
//
// The one pass that lowers a sealed binding plus a topology into the immutable
// solver runtime.
//
//	runtime_binding.go         read form kinds, the runtimeBinding container, catalog freeze
//	runtime_binding_catalog.go graph use rows, the schema Rule ref, carry closures
//	runtime_factor_bind.go     the per-Factor binding pass and its shape matchers
//	runtime_factor.go          the bound Factor vocabulary and its runtime methods
//	runtime_assembly.go        the solver runtime vocabulary and the assembly pass
//	runtime_member.go          the Rule and activation members and their geometry
//	runtime_regions.go         static dependency edges, region binding, demand membership
//	runtime_reindex.go         the immutable lowering of every equation reindex
//	runtime_selected_overlay.go the stale-fenced structural overlay
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
//
// # Execution frame plane
//
// The row-local surface a Rule body sees while it runs, and nothing else.
//
//	runtime_frame.go  the public row API: Row, product session, staging verbs
//	runtime_read.go   typed and staged read runtimes, selection sessions, materialization
//	runtime_output.go output access, typed staging, carry transform, patch accept
//	runtime_rule.go   the bound Rule, its execution and its derivation
//
// # Admission plane
//
// The checker-visible derivation a Rule admission inspects before it accepts.
//
//	rule_admission.go     admission vocabulary, derivation readers, admit path, ticket, evidence
//	rule_derivation_row.go row-level derivation values and the read, target and runtime proofs
//
// # Activation plane
//
// Activation results and the candidates that produce them.
//
//	activation.go                    the activation Product result and its admission
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
// # State and results plane
//
//	state.go one completed immutable Solver result
//
// # Diagnostics plane
//
// Solve-local aggregates and the failure boundary they name. Advisory tiers are
// config-gated, never removed.
//
//	runtime_diagnostics.go the solve-local aggregate diagnostics
//	solve_report.go        the failure reason and the solve report
//
// # Condemned compile plane
//
// Every file below is deleted whole at the receipt flash cut, and state_receipt.go
// is replaced by Snapshot.Query. deletion_manifest_law_test.go is the law: it
// holds the shrink-only manifest and pins each surviving reference into it.
//
//	activation_candidate_issuer.go   receipt_query_admission.go   solver_compiler.go
//	artifact_receipt.go              receipt_rule_admission.go    structural_schedule_certificate.go
//	receipt_observation.go           receipt_solver.go            structural_witness.go
//	schema_query_receipt.go          schema_surface_receipt.go    semantic_directory.go
//	state_receipt.go (replacement, not deletion)
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
