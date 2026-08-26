// Package lower folds a neutral decision region into the physical mask it
// denotes.
//
// A relation decision scope is a sealed decision diagram whose atoms are
// owner-issued identities. The neutral algebra deliberately does not read an
// atom as a physical coordinate, so what physical extent an atom stands for is
// owner knowledge and arrives here as a declaration. This package supplies the
// other half: given the extent of every atom, it evaluates the diagram.
//
// That split is the point. cofiber's closed bootstrap form takes a table keyed
// by whole-region identity, which obliges its caller to predict every
// conjunction the cold proof will visit - an obligation only a fixture world
// can meet. Declaring atoms instead is finite and known, and every region and
// conjunction over them is then answered by evaluation.
//
// The fold is Shannon expansion, so an atom's extent may be any mask rather
// than one physical variable. This package issues no identity, decides no
// scope, and invents no extent: an atom it was not given refuses.
package lower
