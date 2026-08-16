// Package identity is the single root for shared identity vocabulary. Every
// component that must name, compare, or transport an identity takes it from
// here, so identity never acquires a second definition in a leaf package.
//
// # Vocabulary
//
// These words carry exactly one meaning across the analysis tree:
//
//   - ID is a durable identity. Two equal IDs denote the same entity for as
//     long as the entity exists, across processes and across runs.
//   - Locator is a runtime-local address. It is meaningful only relative to
//     one live store instance at one generation.
//   - Generation is a revision fence. It answers whether a runtime-local
//     address is still addressing what it was issued against.
//   - Digest is the hash of a value. A Digest is evidence about content and
//     is not, on its own, the identity of an entity.
//   - Ref is an owner-local relationship. A Ref is resolvable only by its
//     owner and carries no identity outside it.
//
// # Owns
//
// This tree owns MountID, StoreID, Generation, Locator, ContentID, LexicalID,
// and the domain-separated digest construction. The identity tree is the
// dependency leaf every other component may depend on.
//
// # Does not own
//
// This tree does not own Program-local Terms or exact AtomKeys, domain lookup
// keys, artifact row ordinals, binder-local Symbols, or any policy that decides
// which semantic entities exist. The Lua binder defines, issues, and
// interprets Symbol; lowering translates it before publishing canonical
// Program facts. Owners issue identities from the inputs they alone can see;
// consumers carry and compare what they were given. A consumer that needs a
// new identity asks the owner for it rather than deriving one here.
//
// # Cost
//
// Every type here rides hot carry and comparison paths. They are value types
// with no interfaces and no pointers, so equality is a register comparison or
// a fixed-width array comparison and carrying one costs a copy. Derivation is
// the only path that touches a hash and is therefore the only path a caller
// must keep off a hot loop.
//
// # Derivation
//
// Untagged hashing is not reachable through this package. Every derivation
// entry point takes a domain-separation tag and a version, both of which
// enter the preimage before any caller payload, so two owners that hash
// structurally identical payloads under different domains cannot collide. A
// derivation that cannot complete fails closed to the zero identity, which
// reports itself unavailable rather than passing as a real identity.
package identity
