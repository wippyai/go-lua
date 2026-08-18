// Package vocabulary declares the closed vocabulary the target program is written
// in: the operation, callback, subedge, transfer, protocol and boot enums, the
// operation-scoped formal coordinates, the sealed structural ABI handles, and the
// authoring specs Seal consumes.
//
// It references nothing under analysis/program/target. That is the whole reason it
// is a package: the sealed contract is written in this vocabulary, so a declaration
// here must be statable without a contract to read it out of. A vocabulary file
// that needed the contract would be describing a projection, not a declaration.
//
// The direction is stated in the contract package's vocabulary_law_test.go, and the
// compiler enforces it as an import cycle.
package vocabulary
