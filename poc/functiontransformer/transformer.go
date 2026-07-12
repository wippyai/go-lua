// Package functiontransformer proves an exact, deliberately narrow lexical
// function transformer. It is isolated from production.
package functiontransformer

import (
	"fmt"
	"reflect"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Input is syntax-free. Any fact family other than the two listed below makes
// compilation fail closed, including a family added to FactsInput in future.
type Input struct {
	Graph cfg.Graph
	Facts factflow.FactsInput
}

type nodeOp struct {
	point      cfg.Point
	target     pathdom.Path
	source     factflow.ValueSource
	sourcePath pathdom.Path
}

type edgeOp struct {
	to        cfg.Point
	cond      bool
	relations []factflow.BranchPathRelation
}

// Row is one point's complete ordered equation: node operation first, then
// each outgoing edge and its ordered guards. Rows are topologically ordered.
type Row struct {
	Point cfg.Point
	node  *nodeOp
	edges []edgeOp
}

// Transformer is compiled once per nonrecursive lexical CFG.
type Transformer struct {
	graph cfg.Graph
	rows  []Row
}

// Compile verifies the exact supported slice and turns the acyclic CFG into a
// topological equation plan. No concrete parameter value is captured.
func Compile(input Input) (*Transformer, error) {
	if input.Graph == nil {
		return nil, fmt.Errorf("functiontransformer: nil graph")
	}
	if err := rejectUnsupportedFacts(input.Facts); err != nil {
		return nil, err
	}
	order := input.Graph.RPO()
	rank := make(map[cfg.Point]int, len(order))
	for i, point := range order {
		rank[point] = i
	}
	usedExpressions := make(map[factflow.ExprRef]struct{}, len(input.Facts.ExpressionPaths))
	for point, assignment := range input.Facts.PathAssignments {
		if _, ok := rank[point]; !ok {
			return nil, fmt.Errorf("functiontransformer: assignment point %d is outside CFG", point)
		}
		source := assignment.Source()
		if source.HasExpr {
			usedExpressions[source.ExprRef] = struct{}{}
		}
	}
	for expression := range input.Facts.ExpressionPaths {
		if _, used := usedExpressions[expression]; !used {
			return nil, fmt.Errorf("functiontransformer: unused expression path %d", expression)
		}
	}
	for point := range input.Facts.BranchPathRelations {
		if _, ok := rank[point]; !ok || !input.Graph.IsBranch(point) {
			return nil, fmt.Errorf("functiontransformer: relation point %d is not a CFG branch", point)
		}
	}
	for _, edge := range input.Graph.Edges() {
		if rank[edge.From] >= rank[edge.To] {
			return nil, fmt.Errorf("functiontransformer: cyclic CFG edge %d -> %d", edge.From, edge.To)
		}
	}

	rows := make([]Row, 0, len(order))
	for _, point := range order {
		row := Row{Point: point}
		if assignment, ok := input.Facts.PathAssignments[point]; ok {
			source := assignment.Source()
			if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
				return nil, fmt.Errorf("functiontransformer: point %d assignment source is contextual", point)
			}
			sourcePath, ok := input.Facts.ExpressionPaths[source.ExprRef]
			if !ok || sourcePath.IsEmpty() || assignment.TargetPath().IsEmpty() {
				return nil, fmt.Errorf("functiontransformer: point %d assignment lacks structural paths", point)
			}
			row.node = &nodeOp{point: point, target: assignment.TargetPath(), source: source, sourcePath: sourcePath.Clone()}
		}
		for _, successor := range input.Graph.Successors(point) {
			cond, hasCond := input.Graph.EdgeCond(point, successor)
			edge := edgeOp{to: successor, cond: cond}
			for _, relation := range input.Facts.BranchPathRelations[point].Relations() {
				if !hasCond || relation.ActiveOnEdge(cond) {
					edge.relations = append(edge.relations, relation)
				}
			}
			row.edges = append(row.edges, edge)
		}
		rows = append(rows, row)
	}
	return &Transformer{graph: input.Graph, rows: rows}, nil
}

func rejectUnsupportedFacts(input factflow.FactsInput) error {
	value := reflect.ValueOf(input)
	typeOf := value.Type()
	for i := 0; i < value.NumField(); i++ {
		name := typeOf.Field(i).Name
		if name == "PathAssignments" || name == "ExpressionPaths" || name == "BranchPathRelations" {
			continue
		}
		field := value.Field(i)
		nonempty := false
		switch field.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.String:
			nonempty = field.Len() != 0
		default:
			nonempty = !field.IsZero()
		}
		if nonempty {
			return fmt.Errorf("functiontransformer: unsupported fact family %s", name)
		}
	}
	return nil
}

// RootBinding maps a lexical boundary root to a caller-owned structural root.
type RootBinding struct {
	Lexical symbol.ID
	Caller  pathdom.Path
}

// PackedBindings are sorted once and searched without caller maps.
type PackedBindings struct {
	lexical []symbol.ID
	caller  []pathdom.Path
}

func PackBindings(bindings []RootBinding) (PackedBindings, error) {
	copyOf := append([]RootBinding(nil), bindings...)
	sort.Slice(copyOf, func(i, j int) bool { return copyOf[i].Lexical < copyOf[j].Lexical })
	packed := PackedBindings{lexical: make([]symbol.ID, len(copyOf)), caller: make([]pathdom.Path, len(copyOf))}
	for i, binding := range copyOf {
		if binding.Lexical == 0 || binding.Caller.IsEmpty() {
			return PackedBindings{}, fmt.Errorf("functiontransformer: invalid root binding")
		}
		if i != 0 && copyOf[i-1].Lexical == binding.Lexical {
			return PackedBindings{}, fmt.Errorf("functiontransformer: duplicate lexical root %d", binding.Lexical)
		}
		packed.lexical[i], packed.caller[i] = binding.Lexical, binding.Caller.Clone()
	}
	return packed, nil
}

func (b PackedBindings) path(path pathdom.Path) (pathdom.Path, bool) {
	i := sort.Search(len(b.lexical), func(i int) bool { return b.lexical[i] >= path.Symbol })
	if i == len(b.lexical) || b.lexical[i] != path.Symbol {
		return pathdom.Path{}, false
	}
	out := b.caller[i].Clone()
	out.Segments = append(out.Segments, path.Segments...)
	return out, true
}

type boundNode struct {
	point      cfg.Point
	assignment factflow.PathAssignment
	facts      factflow.Facts
}

type boundRelation struct {
	kind        factflow.BranchPathRelationKind
	left, right pathdom.Path
}

type boundEdge struct {
	to        cfg.Point
	cond      bool
	relations []boundRelation
}

type boundRow struct {
	point cfg.Point
	node  *boundNode
	edges []boundEdge
}

// BoundTransformer is a cheap caller-shape instantiation. Parameter values
// remain in EntryState and can vary across executions of the same binding.
type BoundTransformer struct {
	graph cfg.Graph
	rows  []boundRow
}

func (t *Transformer) Bind(bindings PackedBindings) (*BoundTransformer, error) {
	if t == nil {
		return nil, fmt.Errorf("functiontransformer: nil transformer")
	}
	bound := &BoundTransformer{graph: t.graph, rows: make([]boundRow, 0, len(t.rows))}
	for _, row := range t.rows {
		out := boundRow{point: row.Point, edges: make([]boundEdge, 0, len(row.edges))}
		if row.node != nil {
			target, ok := bindings.path(row.node.target)
			if !ok {
				return nil, fmt.Errorf("functiontransformer: unbound target root %d", row.node.target.Symbol)
			}
			sourcePath, ok := bindings.path(row.node.sourcePath)
			if !ok {
				return nil, fmt.Errorf("functiontransformer: unbound source root %d", row.node.sourcePath.Symbol)
			}
			assignment := factflow.NewPathAssignment(target, row.node.source)
			facts := factflow.NewFacts(factflow.FactsInput{
				PathAssignments: map[cfg.Point]factflow.PathAssignment{row.Point: assignment},
				ExpressionPaths: map[factflow.ExprRef]pathdom.Path{row.node.source.ExprRef: sourcePath},
			})
			out.node = &boundNode{point: row.Point, assignment: assignment, facts: facts}
		}
		for _, edge := range row.edges {
			be := boundEdge{to: edge.to, cond: edge.cond}
			for _, relation := range edge.relations {
				left, lok := bindings.path(relation.LeftPath())
				right, rok := bindings.path(relation.RightPath())
				if !lok || !rok {
					return nil, fmt.Errorf("functiontransformer: unbound branch relation at %d", row.Point)
				}
				be.relations = append(be.relations, boundRelation{kind: relation.Kind(), left: left, right: right})
			}
			out.edges = append(out.edges, be)
		}
		bound.rows = append(bound.rows, out)
	}
	return bound, nil
}

// Config supplies caller-owned identities. Resolver and source reads are never
// retained in the lexical transformer.
type Config struct {
	Registry   *axis.Registry
	Resolver   *visibility.Resolver
	EntryState state.State
	Project    factapply.PathTypeProjector
}

// Result contains every point input, matching transfer.Result's convention.
type Result map[cfg.Point]state.State

type pathSources struct {
	registry *axis.Registry
	resolver *visibility.Resolver
	facts    factflow.Facts
}

func (s pathSources) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	path, ok := s.facts.ExpressionPath(source.ExprRef)
	if !ok || s.resolver == nil {
		return product.Value{}, false
	}
	current := in
	if read != nil {
		current = read(point)
	}
	value := current.ReadPathKey(s.registry, s.resolver.KeySpace(), s.resolver.KeyAt(point, path))
	return value, !product.Equal(s.registry, value, product.Bottom(s.registry))
}

// Execute performs one topological pass: each point operation runs once, edge
// guards consume that evolving output, and predecessor contributions join
// before the successor row. It contains no worklist or fixpoint loop.
func (t *BoundTransformer) Execute(config Config) (Result, error) {
	if t == nil || config.Registry == nil || config.Resolver == nil {
		return nil, fmt.Errorf("functiontransformer: incomplete execution config")
	}
	domain := state.Domain(config.Registry)
	result := make(Result, len(t.rows))
	result[t.graph.Entry()] = config.EntryState
	for _, row := range t.rows {
		in, ok := result[row.point]
		if !ok {
			in = domain.Bottom()
			result[row.point] = in
		}
		out := in
		ctx := transfer.NodeContext{Graph: t.graph, Registry: config.Registry, Point: row.point, Node: t.graph.Node(row.point)}
		if row.node != nil {
			sources := pathSources{registry: config.Registry, resolver: config.Resolver, facts: row.node.facts}
			var applied bool
			out, applied = factapply.ApplyConcretePathAssignment(factapply.ConcretePathAssignmentRequest{
				Context: ctx, Resolver: config.Resolver, Facts: row.node.facts, Sources: sources,
				Read: func(cfg.Point) state.State { return in }, Input: in, Output: in, Assignment: row.node.assignment,
			})
			if !applied {
				// Missing source evidence is the production kernel's exact no-op,
				// including an infeasible branch contribution.
				out = in
			}
		}
		for _, edge := range row.edges {
			contribution := out
			edgeCtx := transfer.EdgeContext{Graph: t.graph, Registry: config.Registry, Edge: cfg.Edge{From: row.point, To: edge.to, Cond: edge.cond}, HasCond: t.graph.IsBranch(row.point)}
			for _, relation := range edge.relations {
				contribution = factapply.ApplyConcreteBranchPathRelation(factapply.ConcreteBranchPathRelationRequest{
					Context: edgeCtx, Resolver: config.Resolver, ProjectPath: config.Project,
					Output: contribution, Kind: relation.kind, LeftPath: relation.left, RightPath: relation.right,
				})
			}
			if previous, exists := result[edge.to]; exists {
				result[edge.to] = domain.Join(previous, contribution)
			} else {
				result[edge.to] = contribution
			}
		}
	}
	return result, nil
}
