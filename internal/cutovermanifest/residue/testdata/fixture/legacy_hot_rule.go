// Package fixture is a synthetic residue fixture: a pure-legacy file, used
// only by residue_test.go.
package fixture

// HotRule is legacy protocol residue: the whole file is nothing else.
type HotRule struct{ Ordinal int }

// BindHot wires a HotRule into the old central roster.
func BindHot(r HotRule) {}
