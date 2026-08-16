// Package grammarproof records offline evidence that accepted Lua sources
// exercised every live goyacc production and that the complete witness corpus
// traversed public parse, bind, lower, and sealed Program ingress. It has no
// production dependency: tracing is injected only into a throw-away generated
// parser copy.
//
//go:generate go run ./cmd/generate -root ../../.. -out evidence_gen.go
package grammarproof
