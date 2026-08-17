// Package publication owns the publication relation over the effect, heap,
// pack, and value domains: which exact runtime allocation context was bound
// while a publication authority was live, and membership of selected direct
// allocations in value summaries.
//
// The four member domains are peers; none imports another for this relation,
// so per the composite placement law the relation lives here, and this package
// is the one writer of its result.
//
// The relation is held as three typed evidence paths. An attachment binds one
// selected call effect member to a value summary root before solving, and
// proves one solved subject cell into a membership classification. An
// allocation context event rebuilds the two cross-owner admissions from live
// typed capabilities, reruns that membership proof, and issues one detached
// content-identified record of the proved transition: its subject binding and
// runtime allocation context, the destination context a context-required
// transition carries, and the target effect, escape, mutability, and lifetime
// dispositions the transition declared. Both paths reauthenticate every input
// they are handed, so a caller cannot supply a detached membership scalar,
// context identity, heap key, or target consequence.
//
// The third path binds one mounted branch evidence point to the same Value
// summary surface and reads it back after solving. It carries the same private
// handle discipline: the point coordinates derive the identity that authorizes
// the Engine observation, a second producer naming that point reauthenticates
// its own member through the attachment, and the observation itself is never
// accepted from a caller.
//
// The relation carries no composite.Spec row on the schema composite surface
// and declares no output axis. Both land with the store cut, which brings the
// half a composite is declared against: the typed Frame and its admitted
// write. The deferral is documented at analysis/domain/composite/composite_table.go.
// Until it lands, the relation is composed by its caller, which names the
// exact member coordinates this package reads rather than letting it search a
// plan's artifact inventory.
package publication
