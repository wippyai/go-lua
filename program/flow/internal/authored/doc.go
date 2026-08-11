// Package authored stores the construction-only authored Flow rows for the
// top-level Flow assembly and its internal vertical passes.
//
// No Program/root, Link, artifact, analyzer, domain consumer, or public Flow
// API may retain, return, or query this package. Only top-level Flow assembly
// and its internal vertical passes may import authored; terminal assembly
// consumes its Draft exactly once and retains only the committed authored View
// behind the final Flow authority.
package authored
