// Package scope is a legacy checker scope facade.
package scope

type State struct{}

func NewWithBuiltins() *State {
	return &State{}
}
