package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type snapshotSourceValues struct {
	t              *testing.T
	reg            *axis.Registry
	ks             stateKeyReader
	axis           userlattice.AxisID
	inputMarker    userlattice.ElementID
	outputMarker   userlattice.ElementID
	inputObserved  bool
	outputObserved bool
	value          product.Value
}

type stateKeyReader struct {
	keyspace *visibility.Resolver
	marker   pathdom.Path
	point    cfg.Point
}

func (s *snapshotSourceValues) ValueOfSource(point cfg.Point, _ factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	s.t.Helper()
	marker, ok := visibility.AddressAt(s.ks.keyspace, s.ks.point, s.ks.marker).RootOrVisibleStateKey()
	if !ok {
		s.t.Fatal("marker path is not visible")
	}
	if got, ok := in.ReadUserElement(s.reg, s.ks.keyspace.KeySpace(), s.axis, marker); !ok || got != s.inputMarker {
		s.t.Fatalf("source input marker = %q/%v, want %q/true", got, ok, s.inputMarker)
	}
	s.inputObserved = true
	current := read(point)
	if got, ok := current.ReadUserElement(s.reg, s.ks.keyspace.KeySpace(), s.axis, marker); !ok || got != s.outputMarker {
		s.t.Fatalf("current-point marker = %q/%v, want %q/true", got, ok, s.outputMarker)
	}
	s.outputObserved = true
	return s.value, true
}

func TestApplyConcretePathAssignmentKeepsInputAndEvolvingOutputDistinct(t *testing.T) {
	const taintAxis userlattice.AxisID = "test.concrete.snapshots"
	reg := axis.NewRegistry()
	for _, spec := range []axis.ErasedSpec{
		variantorigin.Spec().Erase(), identity.Spec().Erase(), runtimekind.Spec().Erase(),
		typewitness.Spec().Erase(), escape.Spec().Erase(), evidence.Spec().Erase(), assertion.Spec().Erase(),
	} {
		if err := reg.RegisterErased(spec); err != nil {
			t.Fatalf("register standard value axis: %v", err)
		}
	}
	if _, err := userlattice.Register(reg, testTaintSpec(taintAxis)); err != nil {
		t.Fatalf("register user lattice: %v", err)
	}
	reg.Freeze()

	point := cfg.Point(1701)
	sourceSymbol := symbol.ID(1701)
	targetSymbol := symbol.ID(1702)
	markerSymbol := symbol.ID(1703)
	sourcePath := pathdom.NewPath(sourceSymbol, "source")
	targetPath := pathdom.NewPath(targetSymbol, "target").Field("value")
	markerPath := pathdom.NewPath(markerSymbol, "marker")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1701), HasExpr: true}
	builder := visibility.NewBuilder()
	builder.Define(point, sourceSymbol, "source")
	builder.Define(point, targetSymbol, "target")
	builder.Define(point, markerSymbol, "marker")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	sourceKey := mustStateKeyForPath(t, resolver, point, sourcePath)
	targetKey := mustStateKeyForPath(t, resolver, point, targetPath)
	markerKey := mustStateKeyForPath(t, resolver, point, markerPath)

	input := state.State{}.
		WriteUserElement(reg, ks, taintAxis, sourceKey, "Tainted").
		WriteUserElement(reg, ks, taintAxis, markerKey, "Tainted")
	output := state.State{}.
		WriteUserElement(reg, ks, taintAxis, sourceKey, "Sanitized").
		WriteUserElement(reg, ks, taintAxis, targetKey, "Sanitized").
		WriteUserElement(reg, ks, taintAxis, markerKey, "Sanitized")
	sources := &snapshotSourceValues{
		t:            t,
		reg:          reg,
		ks:           stateKeyReader{keyspace: resolver, marker: markerPath, point: point},
		axis:         taintAxis,
		inputMarker:  "Tainted",
		outputMarker: "Sanitized",
		value:        product.Top(),
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{source.ExprRef: sourcePath},
	})
	got, applied := ApplyConcretePathAssignment(ConcretePathAssignmentRequest{
		Context:    transfer.NodeContext{Registry: reg, Point: point},
		Resolver:   resolver,
		Facts:      facts,
		Sources:    sources,
		Read:       func(cfg.Point) state.State { return state.State{} },
		Input:      input,
		Output:     output,
		Assignment: factflow.NewPathAssignment(targetPath, source),
	})
	if !applied || !sources.inputObserved || !sources.outputObserved {
		t.Fatalf("assignment applied/input/output = %v/%v/%v, want true/true/true", applied, sources.inputObserved, sources.outputObserved)
	}
	if value, ok := got.ReadUserElement(reg, ks, taintAxis, targetKey); !ok || value != "Tainted" {
		t.Fatalf("target user element = %q/%v, want source Input's Tainted/true", value, ok)
	}
	if value, ok := got.ReadUserElement(reg, ks, taintAxis, markerKey); !ok || value != "Sanitized" {
		t.Fatalf("unrelated output marker = %q/%v, want evolving Output's Sanitized/true", value, ok)
	}
}

func TestApplyConcretePathAssignmentRollsBackUnresolvableWrite(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1704)
	target := symbol.ID(1704)
	marker := symbol.ID(1705)
	targetPath := pathdom.NewPath(target, "missing").Field("value")
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1704), HasExpr: true}
	builder := visibility.NewBuilder()
	builder.Define(point, marker, "marker")
	resolver := visibility.NewResolver(builder.Build())
	output := state.State{}.WriteValue(reg, key.SymbolValue(marker), presentValue(reg))
	sources := &recordingSourceValues{values: map[factflow.ValueSource]product.Value{source: absentValue(reg)}}

	got, applied := ApplyConcretePathAssignment(ConcretePathAssignmentRequest{
		Context:    transfer.NodeContext{Registry: reg, Point: point},
		Resolver:   resolver,
		Facts:      factflow.NewFacts(factflow.FactsInput{}),
		Sources:    sources,
		Read:       func(cfg.Point) state.State { return output },
		Input:      state.State{},
		Output:     output,
		Assignment: factflow.NewPathAssignment(targetPath, source),
	})
	if applied {
		t.Fatal("unresolvable path assignment reported applied")
	}
	assertStateEqual(t, reg, got, output)
	assertResolverCall(t, sources, point, source)
}

func TestFactsNodeTransferPathAssignmentCompanionsObserveAssignmentFirst(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(1706)
	target := symbol.ID(1706)
	targetPath := pathdom.NewPath(target, "target").Field("object")
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1706), HasExpr: true}
	entrySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(1707), HasExpr: true}
	rootValue := presentValue(reg)
	entryValue := absentValue(reg)
	sources := &recordingSourceValues{values: map[factflow.ValueSource]product.Value{rootSource: rootValue, entrySource: entryValue}}
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	got := NewFactsNodeTransfer(FactsNodeTransferConfig{
		Facts: factflow.NewFacts(factflow.FactsInput{
			PathAssignments: map[cfg.Point]factflow.PathAssignment{point: factflow.NewPathAssignment(targetPath, rootSource)},
			ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
				rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
					factflow.NewObjectEntryWithMetadata(fieldSuffix("leaf"), entrySource, factflow.SourceSpan{}, ""),
				}),
			},
		}),
		Sources: sources, Visibility: resolver,
	})(transfer.NodeContext{Registry: reg, Point: point}, state.State{})
	assertPathValue(t, reg, resolver.KeySpace(), got, pathdom.PathKey("sym1706@1.object"), rootValue)
	assertPathValue(t, reg, resolver.KeySpace(), got, pathdom.PathKey("sym1706@1.object.leaf"), entryValue)
	if len(sources.calls) != 3 || sources.calls[0].source != rootSource || sources.calls[1].source != rootSource || sources.calls[2].source != entrySource {
		t.Fatalf("source order = %#v, want assignment root then object materialization root and entry", sources.calls)
	}
}

func TestFactsEdgeTransferBranchRelationsConsumeUpdatedState(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)
	a, b, c := symbol.ID(1708), symbol.ID(1709), symbol.ID(1710)
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(a), presentValue(reg)).
		WriteValue(reg, key.SymbolValue(b), product.Top()).
		WriteValue(reg, key.SymbolValue(c), absentValue(reg))
	got := transfer.Run(transfer.Config{
		Graph: graph, Registry: reg, EntryState: initial,
		EdgeTransfer: NewFactsEdgeTransfer(FactsEdgeTransferConfig{Facts: factflow.NewFacts(factflow.FactsInput{
			BranchPathRelations: map[cfg.Point]factflow.BranchPathRelationSet{
				branch: factflow.NewBranchPathRelationSet(
					factflow.NewBranchPathEquality(pathdom.NewPath(a, "a"), pathdom.NewPath(b, "b"), true, false),
					factflow.NewBranchPathEquality(pathdom.NewPath(b, "b"), pathdom.NewPath(c, "c"), true, false),
				),
			},
		})}),
	})
	if !stateIsBottom(reg, got[thenPoint]) {
		t.Fatalf("second branch relation did not observe the first relation's narrowing: a=%s b=%s c=%s",
			formatValue(reg, got[thenPoint].ReadValue(reg, key.SymbolValue(a))),
			formatValue(reg, got[thenPoint].ReadValue(reg, key.SymbolValue(b))),
			formatValue(reg, got[thenPoint].ReadValue(reg, key.SymbolValue(c))))
	}
}
