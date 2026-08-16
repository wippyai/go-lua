package typevalue

import "github.com/wippyai/go-lua/program/keyspace"

// SeedValue is the exact source interpretation for one binder-authorized
// runtime TypeValue seed. It is deliberately the only domain-local operation
// that introduces an Object atom from syntax authority: it returns the
// existing TypeValue root selected by Link and a singleton for the sealed
// descriptor. Flow, aliases, calls, and storage are ordinary Value topology
// and cannot invoke this operation for an unmarked spelling.
func (a *Authority) SeedValue(seed Seed) (Root, Value, bool) {
	root, ok := a.SeedRoot(seed)
	if !ok {
		return Root{}, Value{}, false
	}
	descriptor, ok := a.SeedDescriptor(seed)
	if !ok {
		return Root{}, Value{}, false
	}
	atom, ok := a.Object(descriptor)
	if !ok {
		return Root{}, Value{}, false
	}
	value, ok := a.Singleton(atom)
	if !ok {
		return Root{}, Value{}, false
	}
	return root, value, true
}

// SeedID is the canonical immutable identity of one binder-authorized runtime
// TypeValue source. The source is exactly the existing Boundary Value;
// a TypeValue Rule uses this ID as its operand-content identity instead of
// inventing a source name, descriptor key, or runtime object identity.
func (a *Authority) SeedID(seed Seed) (keyspace.ContentID, bool) {
	if a == nil {
		return keyspace.ContentID{}, false
	}
	return a.SeedValueIdentity(seed)
}
