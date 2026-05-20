// Package nested provides pure helper functions for nested function discovery in Lua.
//
// Lua allows functions to be defined anywhere: as local statements, inside
// table constructors, as method definitions, or as anonymous values. This
// package provides utilities for discovering and classifying these nested
// function definitions.
//
// # Data Types
//
// This package exports data types that describe nested functions:
//   - Child: A transfer-discovered nested function with its identity resolved
//   - FuncInfo: A Child extended with its synthesized function type
//   - ScopeGroup: A group of functions sharing the same parent scope
//
// # Pure Functions
//
// All functions in this package are pure: they take inputs and produce outputs
// without side effects. The orchestration of nested function processing is
// handled by the check package, which uses these helpers.
//
// # Scope Groups
//
// Functions defined at the same lexical level share a scope group. This enables
// mutual recursion: function A can call function B, and B can call A, even if
// B is defined after A in the source code.
//
// # Constructor Pattern
//
// The package detects the Lua OOP constructor pattern (see constructor.go):
//
//	function T.new()
//	    local self = setmetatable({}, T)
//	    self.field = value  -- These become instance fields
//	    return self
//	end
//
// # Self Type Resolution
//
// Helper functions support self-type resolution for methods:
//   - FindTableLiteralOwner: For table literal methods
//   - FindFieldAssignmentBase: For field assignment methods
//   - EnrichSelfTypeWithConstructorFields: For constructor-enriched self
package nested

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
)

// Child holds the resolved identity of a nested function definition.
//
// Each Child represents a function discovered in the parent function's CFG.
// The Child captures the function's definition point, enclosing scope, and
// identity (name, symbol, locality).
type Child struct {
	NF       cfg.NestedFunc
	DefScope *scope.State
	FuncDef  *cfg.FuncDefInfo
	FuncName string
	FuncSym  cfg.SymbolID
	IsLocal  bool
}

// FuncInfo extends Child with data needed during scope group processing.
type FuncInfo struct {
	Child
}

// ScopeGroup holds a group of functions sharing the same parent scope.
//
// Functions in a scope group can see each other's types during analysis,
// enabling mutual recursion. The group is processed as a unit: sibling
// types are computed, then each function is checked with that context.
type ScopeGroup struct {
	Hash     uint64
	Funcs    []*FuncInfo
	MinPoint cfg.Point
}
