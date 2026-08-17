// Package census owns the parser/AST construct census: the denominator of
// every parser alternative, every AST form those alternatives construct, every
// exported carrier of those forms, every representation state those carriers
// admit, every whole-constructor field vector an action builds, and every typed
// slot at which one of those values is consumed. It is cold source evidence
// only. Generation reads parser.go.y and the compiler/ast declarations, and
// never runs a fixture or constructs a Program.
//
// The last two grains are one relation read from its two ends: a product row
// states what a construction builds, and a use row states where each built
// value lands. Together they close it - a construction the parser performs is
// either handed back by its action or consumed at exactly one typed slot.
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
