// Package census owns the parser/AST construct census used by the future
// schema seal disposition join. It is cold source evidence only: generation
// reads parser.go.y and compiler/ast declarations and never runs fixtures or
// constructs a Program.
//
//go:generate go run ./cmd/generate -root ../../../../../ -out census_gen.go
package census
