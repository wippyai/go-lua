// Package runtime is the production transition from an admitted relation to
// its canonical immutable snapshot.
//
// Admission remains responsible for constructing the sealed Ready handoff;
// this package only composes the already-sealed engine runtime and snapshot
// publication stages.
package runtime
