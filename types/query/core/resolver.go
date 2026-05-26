package core

import (
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncResolver adapts field and index lookup functions to a resolver interface.
//
// This enables dependency injection of resolution logic without requiring a
// full Engine instance. Useful for testing and for contexts where only basic
// resolution is needed.
//
// Example usage:
//
//	resolver := &FuncResolver{
//	    FieldFunc: core.Field,
//	    IndexFunc: core.Index,
//	}
type FuncResolver struct {
	// FieldFunc resolves field access (t.name). May be nil.
	FieldFunc func(t typ.Type, name string) (typ.Type, bool)

	// IndexFunc resolves index access (t[key]). May be nil.
	IndexFunc func(t typ.Type, key typ.Type) (typ.Type, bool)
}

// Field delegates to FieldFunc if set, otherwise returns (nil, false).
func (r *FuncResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if r == nil || r.FieldFunc == nil {
		return nil, false
	}
	return r.FieldFunc(t, name)
}

// Index delegates to IndexFunc if set, otherwise returns (nil, false).
func (r *FuncResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r == nil || r.IndexFunc == nil {
		return nil, false
	}
	return r.IndexFunc(t, key)
}

// Resolver returns a FuncResolver using the pure structural lookup functions.
//
// This creates a resolver that uses [Field] and [Index] without memoization.
// Useful when an Engine is not available or for simple one-off queries.
func Resolver() *FuncResolver {
	return &FuncResolver{
		FieldFunc: Field,
		IndexFunc: Index,
	}
}

// QueryResolver adapts memoized TypeOps to the context-free resolver interface
// used by flow/narrowing domains inside a single query evaluation.
type QueryResolver struct {
	ctx *db.QueryContext
	ops TypeOps
}

// NewQueryResolver returns a resolver that routes pure field/index queries
// through the shared query system for the current analysis context.
func NewQueryResolver(ctx *db.QueryContext, ops TypeOps) *QueryResolver {
	if ctx == nil || ops == nil {
		return nil
	}
	return &QueryResolver{ctx: ctx, ops: ops}
}

func (r *QueryResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if r == nil || r.ops == nil || r.ctx == nil {
		return nil, false
	}
	return r.ops.Field(r.ctx, t, name)
}

func (r *QueryResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r == nil || r.ops == nil || r.ctx == nil {
		return nil, false
	}
	return r.ops.Index(r.ctx, t, key)
}
