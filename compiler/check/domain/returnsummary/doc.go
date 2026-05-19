// Package returnsummary owns the return-vector abstract domain.
//
// It canonicalizes, compares, joins, and widens return summaries produced by
// local return inference and interprocedural fact propagation. Orchestration
// packages decide when candidate summaries are produced; this package decides
// how those summaries normalize, refine, merge, and align back to function
// types.
package returnsummary
