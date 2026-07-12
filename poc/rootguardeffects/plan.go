// Package rootguardeffects is an isolated proof of compiling complete root
// writes and value guards into a reusable, acyclic boundary plan.
package rootguardeffects

import (
	"fmt"
	"reflect"
	"sort"

	fixsummary "github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
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

// Admission makes the supported semantic surface measurable. Unsupported
// families are never silently dropped.
type Admission struct {
	Points, RootAssignments, GuardRefinements int
}

type rootOp struct{ assignment factflow.RootAssignment }
type guardOp struct {
	target pathdom.Path
	value  factflow.ValueRefinement
}
type edgeOp struct {
	to     int
	guards []guardOp
}
type row struct {
	point cfg.Point
	root  *rootOp
	edges []edgeOp
}

// Plan is immutable after Compile and contains no caller State.
type Plan struct {
	graph      cfg.Graph
	facts      factflow.Facts
	rows       []row
	returnRoot symbol.ID
	exitIndex  int
	admission  Admission
}

// Compile accepts the complete object/call-free root-write and value-guard
// slice. Reflection makes future FactsInput fields fail closed by default.
func Compile(graph cfg.Graph, input factflow.FactsInput, returnRoot symbol.ID) (*Plan, error) {
	if graph == nil || returnRoot == 0 {
		return nil, fmt.Errorf("rootguardeffects: incomplete input")
	}
	if err := rejectUnsupported(input); err != nil {
		return nil, err
	}
	order := graph.RPO()
	if len(order) > 64 {
		return nil, fmt.Errorf("rootguardeffects: %d points exceed compact plan limit", len(order))
	}
	rank := make(map[cfg.Point]int, len(order))
	for i, point := range order {
		rank[point] = i
	}
	for _, edge := range graph.Edges() {
		if rank[edge.From] >= rank[edge.To] {
			return nil, fmt.Errorf("rootguardeffects: cyclic edge %d -> %d", edge.From, edge.To)
		}
	}
	facts := factflow.NewFacts(input)
	for point, assignment := range input.RootAssignments {
		if _, ok := rank[point]; !ok {
			return nil, fmt.Errorf("rootguardeffects: assignment point %d outside CFG", point)
		}
		source := assignment.Source()
		_, hasPath := input.ExpressionPaths[source.ExprRef]
		if assignment.Kind() != factflow.RootAssignmentOrdinaryRootWrite ||
			len(assignment.TargetPathRef().Segments) != 0 ||
			source.Kind != factflow.ValueSourceExpression || !source.HasExpr || !hasPath {
			return nil, fmt.Errorf("rootguardeffects: contextual root assignment at %d", point)
		}
	}
	for point := range input.BranchRefinements {
		if _, ok := rank[point]; !ok || !graph.IsBranch(point) {
			return nil, fmt.Errorf("rootguardeffects: refinement point %d is not a CFG branch", point)
		}
	}
	rows := make([]row, 0, len(order))
	admission := Admission{Points: len(order)}
	for _, point := range order {
		r := row{point: point}
		if assignment, ok := facts.RootAssignment(point); ok {
			r.root = &rootOp{assignment: assignment}
			admission.RootAssignments++
		}
		for _, successor := range graph.Successors(point) {
			cond, hasCond := graph.EdgeCond(point, successor)
			e := edgeOp{to: rank[successor]}
			for _, refinement := range facts.BranchRefinements(point) {
				value, ok := refinement.ValueForEdge(cond)
				if !hasCond || !ok {
					continue
				}
				if value.NegatedLiteral() || value.FalsyAbsent() {
					return nil, fmt.Errorf("rootguardeffects: contextual guard at %d", point)
				}
				e.guards = append(e.guards, guardOp{target: refinement.TargetPathRef(), value: value})
				admission.GuardRefinements++
			}
			r.edges = append(r.edges, e)
		}
		rows = append(rows, r)
	}
	return &Plan{graph: graph, facts: facts, rows: rows, returnRoot: returnRoot, exitIndex: rank[graph.Exit()], admission: admission}, nil
}

func rejectUnsupported(input factflow.FactsInput) error {
	v := reflect.ValueOf(input)
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		name := t.Field(i).Name
		if name == "RootAssignments" || name == "ExpressionPaths" || name == "BranchRefinements" {
			continue
		}
		field := v.Field(i)
		nonempty := false
		switch field.Kind() {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.String:
			nonempty = field.Len() != 0
		default:
			nonempty = !field.IsZero()
		}
		if nonempty {
			return fmt.Errorf("rootguardeffects: unsupported fact family %s", name)
		}
	}
	return nil
}

func (p *Plan) Admission() Admission { return p.admission }

type Config struct {
	Registry   *axis.Registry
	Resolver   *visibility.Resolver
	Entry      state.State
	StateLanes []state.LaneID
}

type sources struct {
	reg      *axis.Registry
	resolver *visibility.Resolver
	facts    factflow.Facts
}

func (s sources) ValueOfSource(point cfg.Point, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	path, ok := s.facts.ExpressionPath(source.ExprRef)
	if !ok || s.resolver == nil {
		return product.Value{}, false
	}
	current := in
	if read != nil {
		current = read(point)
	}
	value := current.ReadPathKey(s.reg, s.resolver.KeySpace(), s.resolver.KeyAt(point, path))
	return value, !product.Equal(s.reg, value, product.Bottom(s.reg))
}

type ObservationSet map[cfg.Point]struct{}
type Result struct {
	Exit    state.State
	Points  map[cfg.Point]state.State
	Summary fixsummary.Summary
}

// Execute performs one topological pass. Root writes use the complete shared
// transaction, including all object/call sidecars (which compilation currently
// proves absent); guards use the shared ValueRefinement kernel.
func (p *Plan) Execute(config Config, observe ObservationSet) (Result, error) {
	if p == nil || config.Registry == nil || config.Resolver == nil {
		return Result{}, fmt.Errorf("rootguardeffects: incomplete config")
	}
	domain := state.Domain(config.Registry)
	if config.StateLanes != nil {
		domain = state.DomainWithLanes(config.Registry, config.StateLanes)
	}
	entry := config.Entry
	if config.StateLanes != nil {
		entry = state.NormalizeForDomain(domain, entry)
	}
	reachableEmpty := state.Reachable(state.State{})
	if config.StateLanes != nil {
		reachableEmpty = state.NormalizeForDomain(domain, reachableEmpty)
	}
	if !domain.Equal(reachableEmpty, domain.Bottom()) {
		entry = state.Reachable(entry)
		if config.StateLanes != nil {
			entry = state.NormalizeForDomain(domain, entry)
		}
	}
	var values [64]state.State
	present := uint64(1)
	values[0] = entry
	result := Result{}
	if observe != nil {
		result.Points = make(map[cfg.Point]state.State, len(observe))
	}
	source := sources{config.Registry, config.Resolver, p.facts}
	var rootExecutor factapply.ConcreteRootAssignmentPointExecutor
	for index, r := range p.rows {
		in := values[index]
		if present&(uint64(1)<<index) == 0 {
			in = domain.Bottom()
		}
		if _, ok := observe[r.point]; ok {
			result.Points[r.point] = in
		}
		out := in
		ctx := transfer.NodeContext{Graph: p.graph, Registry: config.Registry, Point: r.point, Node: p.graph.Node(r.point)}
		if r.root != nil {
			read := func(cfg.Point) state.State { return in }
			out = rootExecutor.Apply(factapply.ConcreteRootAssignmentPointRequest{
				Context: ctx, Resolver: config.Resolver, Facts: p.facts, Sources: source,
				Read: read, Input: in, Output: out,
			}).Output
		}
		for _, edge := range r.edges {
			contribution := out
			for _, guard := range edge.guards {
				var reachable bool
				contribution, reachable = factapply.ApplyConcreteGuardRefinement(factapply.ConcreteGuardRefinementRequest{
					Registry: config.Registry, Resolver: config.Resolver, Point: r.point,
					Input: out, Output: contribution, TargetPath: guard.target, Refinement: guard.value,
				})
				if !reachable {
					contribution = domain.Bottom()
					break
				}
			}
			if present&(uint64(1)<<edge.to) != 0 {
				values[edge.to] = domain.Join(values[edge.to], contribution)
			} else {
				values[edge.to] = contribution
				present |= uint64(1) << edge.to
			}
		}
	}
	exit := values[p.exitIndex]
	result.Exit = exit
	returned := exit.ReadValue(config.Registry, statekey.SymbolValue(p.returnRoot))
	result.Summary = fixsummary.Normalize(config.Registry, fixsummary.Summary{Returns: []product.Value{returned}})
	return result, nil
}

// SortedObserved provides deterministic test output.
func (r Result) SortedObserved() []cfg.Point {
	out := make([]cfg.Point, 0, len(r.Points))
	for p := range r.Points {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
