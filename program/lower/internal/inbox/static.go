package inbox

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/lower/internal/sourcecoord"
	"github.com/wippyai/go-lua/program/source"
)

type StaticType struct {
	Type ast.TypeExpr
	Host keyspace.Term
	Body keyspace.Term
	Span source.Span
}

type DeclaredCellType struct {
	Type ast.TypeExpr
	Cell keyspace.Term
	Body keyspace.Term
	Span source.Span
}

// Statics owns only crossings that cannot be direct calls without creating a
// semantic package cycle. Ordinary static operations remain direct calls.
type Statics struct {
	phases       *phase.Stack
	types        []StaticType
	declaredCell []DeclaredCellType
}

func NewStatics(phases *phase.Stack) *Statics {
	return &Statics{phases: phases}
}

func (q *Statics) PushType(typ ast.TypeExpr, host, body keyspace.Term, span source.Span) error {
	exact, ok := TypeSpan(typ, span.File)
	if q == nil || q.phases == nil || !ok || host == 0 || body == 0 || span.File == "" || span != exact {
		return fmt.Errorf("programlower: invalid pending static type")
	}
	q.types = append(q.types, StaticType{Type: typ, Host: host, Body: body, Span: span})
	q.phases.Push(phase.StaticType)
	return nil
}

func (q *Statics) PopType() (StaticType, error) {
	if q == nil || len(q.types) == 0 {
		return StaticType{}, fmt.Errorf("programlower: static type token has no payload")
	}
	last := len(q.types) - 1
	request := q.types[last]
	q.types = q.types[:last]
	return request, nil
}

func (q *Statics) PushDeclaredCell(typ ast.TypeExpr, cell, body keyspace.Term, span source.Span) error {
	exact, ok := TypeSpan(typ, span.File)
	if q == nil || q.phases == nil || !ok || cell == 0 || body == 0 || span.File == "" || span != exact {
		return fmt.Errorf("programlower: invalid pending declared Cell type")
	}
	q.declaredCell = append(q.declaredCell, DeclaredCellType{Type: typ, Cell: cell, Body: body, Span: span})
	q.phases.Push(phase.StaticDeclaredCellType)
	return nil
}

// TypeSpan returns one concrete type expression's exact structural span. It
// validates typed nil but makes no semantic type judgment.
func TypeSpan(typ ast.TypeExpr, file string) (source.Span, bool) {
	var holder ast.PositionHolder
	switch node := typ.(type) {
	case *ast.AnnotatedTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.PrimitiveTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.OptionalTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.UnionTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.IntersectionTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.ArrayTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.MapTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.RecordTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.FunctionTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.AssertsTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.TypeRefExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.GenericTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.LiteralTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.TypeOfExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.KeyOfExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.IndexAccessExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	case *ast.ConditionalTypeExpr:
		if node == nil {
			return source.Span{}, false
		}
		holder = node
	default:
		return source.Span{}, false
	}
	if file == "" {
		return source.Span{}, false
	}
	span, ok := sourcecoord.Build(file, holder.Line(), holder.Column(), holder.LastLine(), holder.LastColumn())
	if !ok {
		return source.Span{}, false
	}
	return span, true
}

func (q *Statics) PopDeclaredCell() (DeclaredCellType, error) {
	if q == nil || len(q.declaredCell) == 0 {
		return DeclaredCellType{}, fmt.Errorf("programlower: declared Cell type token has no payload")
	}
	last := len(q.declaredCell) - 1
	request := q.declaredCell[last]
	q.declaredCell = q.declaredCell[:last]
	return request, nil
}

func (q *Statics) Clean() bool {
	return q != nil && len(q.types) == 0 && len(q.declaredCell) == 0
}
