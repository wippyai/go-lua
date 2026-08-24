// Package fixture is a synthetic fixture used only by render_test.go: a
// pure-legacy file exercising the legacy-files-to-remove section.
package fixture

// HotRule is legacy protocol residue: the whole file is nothing else.
type HotRule struct{ Ordinal int }

// BindHot wires a HotRule into the old central roster.
func BindHot(r HotRule) {}
