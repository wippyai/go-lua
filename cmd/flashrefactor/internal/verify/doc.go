// Package verify checks only structural consequences of a flash-refactor lock.
//
// It has no filesystem, type-checker, resolver, or test-runner dependency. The
// executor supplies exact before/after source and import-graph snapshots, while
// semantic evidence (diagnostics and resolved-object residue) stays owned by
// the semantic package. That boundary keeps structural gates from becoming a
// second semantic authority.
package verify
