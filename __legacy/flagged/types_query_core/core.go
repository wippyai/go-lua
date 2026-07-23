// Package core is a legacy query-engine facade retained for Wippy integration.
package core

type Engine struct{}

type FuncResolver struct {
	FieldFunc any
	IndexFunc any
}

func NewEngineWithStdlib(_ any) *Engine {
	return &Engine{}
}

func Field(_ ...any) any { return nil }

func Index(_ ...any) any { return nil }
