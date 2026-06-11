// Package semantics extracts AST semantic facts owned by cfgbuild statement points.
package semantics

import "errors"

var (
	ErrNoCFG         = errors.New("semantics: missing cfg")
	ErrPointMismatch = errors.New("semantics: statement point count mismatch")
)
