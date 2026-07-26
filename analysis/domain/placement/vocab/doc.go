// Package vocab owns the name-frozen placement and escape vocabulary shared by
// analysis, manifests, placement facts, and compiled runtime artifacts.
//
// The names in this package are the in-memory contract.  Wire codecs translate
// explicitly and must not depend on the numeric values below.  This is
// especially important for equation's compiled-artifact runtime projection,
// whose byte ordinals predate this package and face the JIT contract.
//
// # Inventory and displacement map
//
// The pre-unification value sets and their exact canonical mappings are:
//
//	Previous spelling                         Previous values                                      Canonical mapping
//	signature.EscapeKind                      None/Borrow/Retain/Store/Send/Export/Opaque          Escape: same names
//	escapeevent.Kind                          None/Borrow/Retain/Store/Send/Export/Opaque          Escape: same names
//	placement.EscapeTransition                None/Return/Retain/Store/Send/Export/Opaque          Escape: same names; Borrow has no transition
//	placement.Value                           Bottom/Stack/OwnedHeap/SharedHeap/Unknown             Placement: same names
//	equation.RuntimeEscape                    Unknown/None/Return/Store/Share                       Escape: Opaque/None/Return/Store/Send
//	equation.RuntimePlacement                 Unknown/Interpreter/Stack/Register/OwnedHeap/SharedHeap Placement: same names
//	signature.PlacementConsequence            keep/owned-heap/shared-heap                          Consequence: Keep/OwnedHeap/SharedHeap
//	placement/event fact label                owned/shared/sealed/environment/suspended            Event: Owned/Shared/Sealed/Environment/Suspended
//	placement/contract fact label             borrow/retain/send/mutate/metatable/identity/         Boundary: Borrow/Retain/Send/Mutate/Metatable/Identity/
//	                                           dynamic-index/iterator/local/contains                           DynamicIndex/Iterator/Local/Contains
//
// RuntimeEscape's "Share" is the same transfer class called Send by the richer
// manifest vocabulary.  RuntimeEscape's "Unknown" is the conservative opaque
// escape class.  These renamed runtime values retain their historical byte
// ordinals through explicit wire conversion.
//
// # Escape meanings
//
//   - None: no escape event is present.  It is a zero/sentinel, not proof that
//     an allocation is stack resident.
//   - Borrow: a callee may observe a value only for the call's duration and
//     does not retain its reachable graph.
//   - Retain: a callee keeps a reachable graph beyond the call.
//   - Store: a graph is installed into another owner or escaping root.
//   - Send: a graph crosses a sharing or actor/channel transfer boundary.
//   - Export: a graph crosses a module or externally visible boundary.
//   - Opaque: an unknown boundary may retain or share the graph.
//   - Return: a graph crosses a function return boundary.  This variant existed
//     only in placement.EscapeTransition; manifest summaries spell a returned
//     parameter as Export with ThroughReturn set.
//
// Retain, Store, and Return all require OwnedHeap in the placement transition
// policy.  Send, Export, and Opaque all require SharedHeap.  These duplicate
// consequences are intentional: the variants preserve why a transition
// occurred.  Borrow has no placement transition, and None is a sentinel.
//
// # Placement meanings
//
//   - Bottom: unreachable analysis state.  It is not serialized as a runtime
//     placement and must not be silently treated as Unknown.
//   - Stack: frame-local storage in the current activation.
//   - OwnedHeap: heap storage with one actor or analysis owner.
//   - SharedHeap: heap storage that may be shared or externally observed.
//   - Unknown: conservative placement with no optimization permission.
//   - Interpreter: JIT execution remains in interpreter-managed storage.
//   - Register: JIT-proven register residency.
//
// Interpreter and Register have no analysis-lattice counterparts; Bottom has
// no runtime-artifact counterpart.  They are reachable on only one side of the
// analysis/JIT boundary and are documented rather than discarded.
//
// # Placement fact labels
//
// Owned and Shared are the fact-level projections of the escape consequences
// above.  They deliberately do not recover a lost cause: an "owned" fact may
// have arisen from Retain, Store, or Return, and a "shared" fact may have
// arisen from Send, Export, or Opaque.  Sealed, Environment, and Suspended are
// orthogonal lifecycle/lifetime evidence, not additional escape enum members.
// Boundary labels identify the contract which discharged an opaque-call
// fallback; only Borrow, Retain, and Send overlap the escape vocabulary.
package vocab
