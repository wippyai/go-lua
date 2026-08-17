// Package census owns the parser/AST construct census: the denominator of
// every parser alternative, every AST form those alternatives construct, every
// exported carrier of those forms, and every representation state those
// carriers admit. It is cold source evidence only. Generation reads parser.go.y
// and the compiler/ast declarations, and never runs a fixture or constructs a
// Program.
//
// Laws stated over those rows live here too, so a law whose premises are rows
// stays with the rows: the structural induction premises are one such law.
//
// The census is consumed by the sealed catalog's grammar disposition join,
// which accounts for each row. It lives outside the internal tree deliberately:
// the account is stated at the seal, so a denominator fenced to the frontend
// could not reach it.
//
//go:generate go run ./cmd/generate -root ../../../ -out census_gen.go
package census
