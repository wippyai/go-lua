package typepresentation

import (
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TypePair is the eager representation: presentation and semantic nodes are
// materialized together bottom-up, so selecting either graph is O(1).
type TypePair struct {
	Presentation typ.Type
	Semantic     typ.Type
}

type PairedField struct {
	Name     string
	Type     TypePair
	Optional bool
	Readonly bool
}

func PairFunction(fn *typ.Function) TypePair {
	if fn == nil {
		return TypePair{}
	}
	return TypePair{Presentation: fn, Semantic: fn.SemanticType()}
}

func PairRecord(fields []PairedField) TypePair {
	presented := make([]typ.Field, len(fields))
	semantic := make([]typ.Field, len(fields))
	for i, field := range fields {
		presented[i] = typ.Field{Name: field.Name, Type: field.Type.Presentation, Optional: field.Optional, Readonly: field.Readonly}
		semantic[i] = typ.Field{Name: field.Name, Type: field.Type.Semantic, Optional: field.Optional, Readonly: field.Readonly}
	}
	return TypePair{
		Presentation: typ.RebuildRecord(typ.RecordParts{Fields: presented}),
		Semantic:     typ.RebuildRecord(typ.RecordParts{Fields: semantic}),
	}
}

func PairUnion(members ...TypePair) TypePair {
	presented := make([]typ.Type, len(members))
	semantic := make([]typ.Type, len(members))
	for i, member := range members {
		presented[i] = member.Presentation
		semantic[i] = member.Semantic
	}
	return TypePair{Presentation: typ.MaterializeUnion(presented), Semantic: typ.MaterializeUnion(semantic)}
}

// PairedRecursive creates both placeholders before either body is installed,
// preserving recursive identity in both graphs without a post-construction
// traversal.
type PairedRecursive struct {
	presentation *typ.Recursive
	semantic     *typ.Recursive
}

func NewPairedRecursive(name string) *PairedRecursive {
	return &PairedRecursive{
		presentation: typ.NewRecursivePlaceholder(name),
		semantic:     typ.NewRecursivePlaceholder(name),
	}
}

func (r *PairedRecursive) Refs() TypePair {
	if r == nil {
		return TypePair{}
	}
	return TypePair{Presentation: r.presentation, Semantic: r.semantic}
}

func (r *PairedRecursive) SetBody(body TypePair) {
	if r == nil {
		return
	}
	r.presentation.SetBody(body.Presentation)
	r.semantic.SetBody(body.Semantic)
}

type semanticBox struct{ value typ.Type }

// LazySemanticGraph stores only the presentation root until semantic selection
// is requested. Selection is O(1) after the first graph walk.
type LazySemanticGraph struct {
	presentation typ.Type
	semantic     atomic.Pointer[semanticBox]
}

// SemanticGraphCache is an owner-scoped cache for presentation roots. It is
// intended to live beside a manifest or analysis-database cache: callers can
// prewarm roots before publishing values to semantic consumers, without a
// global cache or one lazy pointer per composite node.
type SemanticGraphCache struct {
	mu     sync.RWMutex
	graphs map[typ.Type]*LazySemanticGraph
}

func NewSemanticGraphCache() *SemanticGraphCache {
	return &SemanticGraphCache{graphs: make(map[typ.Type]*LazySemanticGraph)}
}

func (c *SemanticGraphCache) Graph(root typ.Type) *LazySemanticGraph {
	if c == nil {
		return NewLazySemanticGraph(root)
	}
	c.mu.RLock()
	graph := c.graphs[root]
	c.mu.RUnlock()
	if graph != nil {
		return graph
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if graph = c.graphs[root]; graph == nil {
		graph = NewLazySemanticGraph(root)
		c.graphs[root] = graph
	}
	return graph
}

func (c *SemanticGraphCache) Semantic(root typ.Type) typ.Type {
	return c.Graph(root).Semantic()
}

// Prewarm materializes and publishes each root's semantic graph. Duplicate
// roots are naturally coalesced by Graph.
func (c *SemanticGraphCache) Prewarm(roots ...typ.Type) {
	for _, root := range roots {
		c.Semantic(root)
	}
}

func NewLazySemanticGraph(presentation typ.Type) *LazySemanticGraph {
	return &LazySemanticGraph{presentation: presentation}
}

func (g *LazySemanticGraph) Presentation() typ.Type {
	if g == nil {
		return nil
	}
	return g.presentation
}

func (g *LazySemanticGraph) Semantic() typ.Type {
	if g == nil {
		return nil
	}
	if box := g.semantic.Load(); box != nil {
		return box.value
	}
	candidate := &semanticBox{value: semanticizeGraph(g.presentation, make(map[typ.Type]typ.Type))}
	if g.semantic.CompareAndSwap(nil, candidate) {
		return candidate.value
	}
	return g.semantic.Load().value
}

func semanticizeGraph(t typ.Type, memo map[typ.Type]typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if got := memo[t]; got != nil {
		return got
	}
	switch node := t.(type) {
	case *typ.Function:
		typeParams := semanticizeTypeParams(node.TypeParams, memo)
		childrenChanged := len(typeParams) != len(node.TypeParams)
		for i := range typeParams {
			childrenChanged = childrenChanged || typeParams[i] != node.TypeParams[i]
		}
		params := make([]typ.Param, len(node.Params))
		for i, param := range node.Params {
			projectedType := semanticizeGraph(param.Type, memo)
			childrenChanged = childrenChanged || projectedType != param.Type
			param.Type = projectedType
			param.Name = ""
			if param.Receiver {
				param.Name = "self"
			}
			params[i] = param
		}
		returns := make([]typ.Type, len(node.Returns))
		for i, result := range node.Returns {
			returns[i] = semanticizeGraph(result, memo)
			childrenChanged = childrenChanged || returns[i] != result
		}
		variadic := semanticizeGraph(node.Variadic, memo)
		childrenChanged = childrenChanged || variadic != node.Variadic
		if !childrenChanged {
			semantic := node.SemanticType()
			memo[t] = semantic
			return semantic
		}
		semantic := typ.RebuildFunction(typ.FunctionParts{
			TypeParams: typeParams,
			Params:     params,
			Variadic:   variadic,
			Returns:    returns,
		})
		memo[t] = semantic
		return semantic
	case *typ.Record:
		fields := make([]typ.Field, len(node.Fields))
		for i, field := range node.Fields {
			field.Type = semanticizeGraph(field.Type, memo)
			fields[i] = field
		}
		staticMembers := make([]typ.StaticMember, len(node.StaticMembers))
		for i, member := range node.StaticMembers {
			member.Type = semanticizeGraph(member.Type, memo)
			staticMembers[i] = member
		}
		semantic := typ.RebuildRecord(typ.RecordParts{
			Fields:        fields,
			StaticMembers: staticMembers,
			Metatable:     semanticizeGraph(node.Metatable, memo),
			MapKey:        semanticizeGraph(node.MapKey, memo),
			MapValue:      semanticizeGraph(node.MapValue, memo),
			Open:          node.Open,
			AssumeSorted:  true,
		})
		memo[t] = semantic
		return semantic
	case *typ.Union:
		members := make([]typ.Type, len(node.Members))
		for i, member := range node.Members {
			members[i] = semanticizeGraph(member, memo)
		}
		semantic := typ.MaterializeUnion(members)
		memo[t] = semantic
		return semantic
	case *typ.Intersection:
		members := make([]typ.Type, len(node.Members))
		for i, member := range node.Members {
			members[i] = semanticizeGraph(member, memo)
		}
		semantic := typ.MaterializeIntersection(members)
		memo[t] = semantic
		return semantic
	case *typ.Optional:
		semantic := typ.MaterializeOptional(semanticizeGraph(node.Inner, memo))
		memo[t] = semantic
		return semantic
	case *typ.Array:
		semantic := typ.NewArray(semanticizeGraph(node.Element, memo))
		memo[t] = semantic
		return semantic
	case *typ.Map:
		semantic := typ.NewMap(semanticizeGraph(node.Key, memo), semanticizeGraph(node.Value, memo))
		memo[t] = semantic
		return semantic
	case *typ.ReadonlyMap:
		semantic := typ.NewReadonlyMap(semanticizeGraph(node.Key, memo), semanticizeGraph(node.Value, memo))
		memo[t] = semantic
		return semantic
	case *typ.Tuple:
		elements := make([]typ.Type, len(node.Elements))
		for i, element := range node.Elements {
			elements[i] = semanticizeGraph(element, memo)
		}
		semantic := typ.NewTuple(elements...)
		memo[t] = semantic
		return semantic
	case *typ.Meta:
		semantic := typ.NewMeta(semanticizeGraph(node.Of, memo))
		memo[t] = semantic
		return semantic
	case *typ.Alias:
		semantic := typ.NewAlias(node.Name, semanticizeGraph(node.Target, memo))
		memo[t] = semantic
		return semantic
	case *typ.Annotated:
		semantic := typ.NewAnnotated(semanticizeGraph(node.Inner, memo), node.Annotations)
		memo[t] = semantic
		return semantic
	case *typ.Interface:
		methods := make([]typ.Method, len(node.Methods))
		for i, method := range node.Methods {
			projected, ok := semanticizeGraph(method.Type, memo).(*typ.Function)
			if !ok {
				panic("semantic projection: interface method is not a function")
			}
			methods[i] = typ.Method{Name: method.Name, Type: projected}
		}
		semantic := typ.NewInterface(node.Name, methods)
		memo[t] = semantic
		return semantic
	case *typ.Generic:
		// Publish the generic placeholder before walking constraints or its body;
		// both may refer back to this declaration.
		semantic := typ.NewGeneric(node.Name, nil, nil)
		memo[t] = semantic
		semantic.TypeParams = semanticizeTypeParams(node.TypeParams, memo)
		semantic.SetBody(semanticizeGraph(node.Body, memo))
		return semantic
	case *typ.Instantiated:
		generic, ok := semanticizeGraph(node.Generic, memo).(*typ.Generic)
		if !ok {
			panic("semantic projection: instantiated base is not generic")
		}
		args := make([]typ.Type, len(node.TypeArgs))
		for i, arg := range node.TypeArgs {
			args[i] = semanticizeGraph(arg, memo)
		}
		semantic := typ.Instantiate(generic, args...)
		memo[t] = semantic
		return semantic
	case *typ.TypeParam:
		semantic := typ.NewTypeParam(node.Name, semanticizeGraph(node.Constraint, memo))
		memo[t] = semantic
		return semantic
	case *typ.Recursive:
		placeholder := typ.NewRecursivePlaceholder(node.Name)
		memo[t] = placeholder
		placeholder.SetBody(semanticizeGraph(node.Body, memo))
		return placeholder
	default:
		// Primitive, literal, Ref, and other childless immutable nodes are
		// already semantic and safe to share.
		memo[t] = t
		return t
	}
}

func semanticizeTypeParams(params []*typ.TypeParam, memo map[typ.Type]typ.Type) []*typ.TypeParam {
	if len(params) == 0 {
		return nil
	}
	semantic := make([]*typ.TypeParam, len(params))
	for i, param := range params {
		if existing, ok := memo[param].(*typ.TypeParam); ok {
			semantic[i] = existing
			continue
		}
		projected := typ.NewTypeParam(param.Name, semanticizeGraph(param.Constraint, memo))
		memo[param] = projected
		semantic[i] = projected
	}
	return semantic
}
