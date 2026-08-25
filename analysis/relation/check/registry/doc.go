// Package registry owns the one read-only index of an unchecked execution
// schema used by the independent checker passes.  It does not decide any
// proof law: it only preserves declarations, canonical lookup order, and
// structural defects that all passes would otherwise rediscover.
package registry
