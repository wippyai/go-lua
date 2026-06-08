// Package indexread projects indexed read results through solved flow proofs.
package indexread

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	flowfacts "github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// Flow is the solved proof surface needed to refine indexed reads.
type Flow = flowfacts.IndexReadObservationProofs

// PathOf maps an expression to its flow path at the read point.
type PathOf func(ast.Expr) constraint.Path

// Query describes one indexed read projection.
type Query struct {
	Point     cfg.Point
	View      flowfacts.PathReadView
	Container typ.Type
	Result    typ.Type
	Object    ast.Expr
	Key       ast.Expr
	KeyType   typ.Type
	Flow      Flow
	PathOf    PathOf
}

// ContextQuery lowers one indexed-read expression into an AST-free proof
// context.
type ContextQuery struct {
	Container typ.Type
	Object    ast.Expr
	Key       ast.Expr
	KeyType   typ.Type
	PathOf    PathOf
}

// Refine returns a result type refined by solved index-read proofs.
func Refine(q Query) (typ.Type, bool) {
	index, ok := Context(ContextQuery{
		Container: q.Container,
		Object:    q.Object,
		Key:       q.Key,
		KeyType:   q.KeyType,
		PathOf:    q.PathOf,
	})
	if !ok {
		return nil, false
	}
	return flowfacts.RefineIndexReadObservation(flowfacts.IndexReadObservationQuery{
		Point:  q.Point,
		View:   q.View,
		Result: q.Result,
		Index:  index,
		Proofs: q.Flow,
	})
}

// Context returns an AST-free indexed-read context. The returned context is
// usable when at least one proof shape was recognized.
func Context(q ContextQuery) (flowfacts.PathObservationIndexRead, bool) {
	var out flowfacts.PathObservationIndexRead
	out.Container = q.Container
	out.KeyType = q.KeyType
	if literal, ok := literalKeyType(q.Key); ok {
		out.KeyType = literal
	}
	if q.PathOf != nil {
		out.TablePath = q.PathOf(q.Object)
		out.KeyPath = q.PathOf(q.Key)
	}
	if path, offset, ok := indexVarOffsetPathFromExpr(q.Key, q.PathOf); ok {
		out.IndexVarPath = path
		out.IndexVarOffset = offset
		out.HasIndexVar = true
	}
	if path, offset, ok := lenIndexPathFromExpr(q.Key, q.PathOf); ok {
		out.LengthPath = path
		out.LengthOffset = offset
		out.HasLength = true
	}
	if index, ok := integerLiteralIndex(q.Key); ok {
		out.LiteralIndex = index
		out.HasLiteralIndex = true
	}
	return out, !out.TablePath.IsEmpty() || !out.KeyPath.IsEmpty() || out.HasIndexVar || out.HasLength || out.HasLiteralIndex
}

func literalKeyType(expr ast.Expr) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.StringExpr:
		return typ.LiteralString(e.Value), true
	case *ast.NumberExpr:
		n, ok := numparse.ParseIntegerLiteral(e.Value)
		if ok {
			return typ.LiteralInt(n), true
		}
	}
	return nil, false
}

func integerLiteralIndex(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func indexVarOffsetPathFromExpr(expr ast.Expr, paths PathOf) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return constraint.Path{}, 0, false
		}
		if paths != nil {
			path := paths(e)
			if !path.IsEmpty() {
				return path, 0, true
			}
		}
		return constraint.Path{Root: e.Value}, 0, true
	case *ast.ArithmeticOpExpr:
		ident, ok := e.Lhs.(*ast.IdentExpr)
		if !ok || ident.Value == "" {
			return constraint.Path{}, 0, false
		}
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		if paths != nil {
			path := paths(ident)
			if !path.IsEmpty() {
				return path, k, true
			}
		}
		return constraint.Path{Root: ident.Value}, k, true
	}
	return constraint.Path{}, 0, false
}

func lenIndexPathFromExpr(expr ast.Expr, paths PathOf) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.UnaryLenOpExpr:
		path := paths(e.Expr)
		return path, 0, !path.IsEmpty()
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		path, offset, ok := lenIndexPathFromExpr(e.Lhs, paths)
		if !ok {
			return constraint.Path{}, 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return path, offset + k, true
	}
	return constraint.Path{}, 0, false
}

func intConstFromExpr(expr ast.Expr) (int64, bool) {
	switch v := expr.(type) {
	case *ast.NumberExpr:
		return numparse.ParseIntegerLiteral(v.Value)
	case *ast.UnaryMinusOpExpr:
		if n, ok := intConstFromExpr(v.Expr); ok {
			return -n, true
		}
	}
	return 0, false
}
