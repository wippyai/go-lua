// Package typestate owns the judgment a protocol declaration is decided by:
// the protocol and state names, the FSM definition and its well-formedness,
// the abstract state lattice a solve carries, the obligation set a lifecycle
// label is discharged against, and the closed verdict vocabulary every
// typestate question is answered in.
//
// All of it is a value algebra. A definition is authored in a manifest and
// decoded once at the module boundary; an Abstract is a lattice element a
// carrier holds; a Verdict is a decision drawn from the two. Nothing in this
// package holds a coordinate, names a declaration surface, or runs during a
// fixpoint - which is exactly why it can be read from the module boundary, the
// signature vocabulary, and the solver alike.
//
// # Where the declaration rows live
//
// This package declares no row on any surface of the analyzer declaration
// table, and its children declare all of them. The split is the altitude
// doctrine applied to one domain: the judgment is the core, the owner surface
// is downstream of it, and a core that named a surface would make every
// consumer of the vocabulary - including the portable manifest boundary -
// depend on the analyzer's declaration table.
//
// The children and what each owns:
//
//   - statecell owns the coordinate space typestate facts are solved over: the
//     dense product of Heap's allocation-root directory with the sealed
//     protocol directory. One cell is the state of one resource under one
//     protocol; the program point is the engine's dimension, not the space's.
//   - program owns the rule declaration: the candidate relation over call
//     occurrences that carry a callable obligation, the reads that resolve the
//     receiver's resource and its current state, and the fold that draws the
//     verdict and publishes the successor state.
//
// The zero-row statement about this package is executable: a surface of the
// declaration table is reached by importing its package, and a peer domain by
// importing it, so a core that declares no row and holds no inter-domain edge
// imports no module package at all. The declaration law states exactly that,
// over this package's own sources; a row added here cannot be added without
// this file being rewritten in the same change, and a row that belongs to a
// child cannot be smuggled in here at all.
//
// # Consumers
//
// The FSM definition and the obligation vocabulary are consumed at the module
// boundary, where a manifest is decoded and its lifecycle labels are checked
// against the protocols the same manifest declares, and by the lifecycle
// effect labels that carry a protocol and a state as their payload. The
// abstract lattice and the verdicts are consumed by the children above and by
// the diagnostic rows their verdict column is published through.
package typestate
