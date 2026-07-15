package transformer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSparseProjectionTraceCapturesExactPointEdgeAndReturnWorlds(t *testing.T) {
	graph, branch, left, right, ret, requirements := sparseProjectionReturnFixture(t)
	arena := NewArena(standard.Registry())
	param := arena.Root(Root{Kind: RootParam})
	builder, err := newSparseProjectionTraceBuilder(arena, requirements)
	if err != nil {
		t.Fatal(err)
	}
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		t.Fatal(err)
	}
	exit, err := solveExactWTOCFGExpandedExitRowsWithTrace(context.Background(), graph, tape, arena,
		SymbolicCFGRow{Guard: arena.True()},
		func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
			if point == ret {
				row.Operations = append(row.Operations, Operation{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param})
			}
			return []SymbolicCFGRow{row}, nil
		},
		func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
			if point != branch {
				t.Fatalf("unexpected branch point %d", point)
			}
			if cond {
				return row, arena.Truthy(param), nil
			}
			return row, arena.Falsy(param), nil
		}, SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{Shape: Shape{Params: 1}}}, builder)
	if err != nil || len(exit) != 2 {
		t.Fatalf("exit rows/error = %d/%v, want 2/nil", len(exit), err)
	}
	trace, err := builder.freeze()
	if err != nil {
		t.Fatal(err)
	}
	if trace.schema != requirements.SchemaID() || trace.inventory != requirements.ConsumerInventoryID() || len(trace.slots) != len(requirements.Entries(false)) {
		t.Fatalf("trace identity/slots = %x/%x/%d", trace.schema, trace.inventory, len(trace.slots))
	}
	wantPointGuards := map[cfg.Point]Guard{
		graph.Entry(): arena.True(), branch: arena.True(), left: arena.Truthy(param), right: arena.Falsy(param),
		ret: arena.Or(arena.Truthy(param), arena.Falsy(param)), graph.Exit(): arena.Or(arena.Truthy(param), arena.Falsy(param)),
	}
	wantEdges := map[sparseProjectionEdge]Guard{
		{from: branch, to: left}:  arena.Truthy(param),
		{from: branch, to: right}: arena.Falsy(param),
	}
	returnFragments := 0
	for _, slot := range trace.slots {
		switch slot.requirement.Stage() {
		case operationplan.RequirementPoint:
			if want := wantPointGuards[slot.requirement.Point()]; slot.guard != want {
				t.Fatalf("point %d guard = %s, want %s", slot.requirement.Point(), arena.canonicalGuard(slot.guard), arena.canonicalGuard(want))
			}
		case operationplan.RequirementEdge:
			to, _ := slot.requirement.EdgeTarget()
			if want := wantEdges[sparseProjectionEdge{from: slot.requirement.Point(), to: to}]; slot.guard != want {
				t.Fatalf("edge %d->%d guard = %s, want %s", slot.requirement.Point(), to, arena.canonicalGuard(slot.guard), arena.canonicalGuard(want))
			}
		case operationplan.RequirementBoundary:
			if fact, ok := slot.requirement.FactKind(); !ok || fact != operationplan.Return || len(slot.fragments) != 2 {
				t.Fatalf("return fragments/fact = %d/%v/%v", len(slot.fragments), fact, ok)
			}
			for _, fragment := range slot.fragments {
				if len(fragment.operations) != 1 || fragment.operations[0].Value != param ||
					(fragment.guard != arena.Truthy(param) && fragment.guard != arena.Falsy(param)) {
					t.Fatalf("uncorrelated return fragment: %#v", fragment)
				}
				returnFragments++
			}
		}
	}
	if returnFragments != 2 {
		t.Fatalf("return fragments = %d, want 2", returnFragments)
	}
}

func TestSparseProjectionTraceRejectsUnsupportedBoundaryBeforeExecution(t *testing.T) {
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	lowered := wir.NewBody("unsupported-projection")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 41
	plan := operationplan.New(graph, factflow.FactsInput{PathAssignments: map[cfg.Point]factflow.PathAssignment{assign: {}}}).WithObservationIdentity(owner, lowered, graph)
	requirements, sealed := plan.ObservationRequirements()
	if !sealed {
		t.Fatal("requirements not sealed")
	}
	if builder, err := newSparseProjectionTraceBuilder(NewArena(standard.Registry()), requirements); builder != nil || err == nil || !strings.Contains(err.Error(), "body.boundary.path-assignment.v1") {
		t.Fatalf("unsupported builder/error = %#v/%v", builder, err)
	}
}

func TestSparseProjectionTraceFreezeIsDeterministicUnderRowPermutation(t *testing.T) {
	_, _, _, _, ret, requirements := sparseProjectionReturnFixture(t)
	arena := NewArena(standard.Registry())
	param := arena.Root(Root{Kind: RootParam})
	rows := []SymbolicCFGRow{
		{Guard: arena.Truthy(param), Operations: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param}}},
		{Guard: arena.Falsy(param), Operations: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param}}},
	}
	freeze := func(order []int) *sparseProjectionTrace {
		builder, err := newSparseProjectionTraceBuilder(arena, requirements)
		if err != nil {
			t.Fatal(err)
		}
		for _, index := range order {
			builder.pointOutput(ret, rows[index])
		}
		trace, err := builder.freeze()
		if err != nil {
			t.Fatal(err)
		}
		return trace
	}
	left, right := freeze([]int{0, 1}), freeze([]int{1, 0})
	if sparseProjectionTraceTestKey(arena, left) != sparseProjectionTraceTestKey(arena, right) {
		t.Fatalf("permuted traces differ:\n%s\n%s", sparseProjectionTraceTestKey(arena, left), sparseProjectionTraceTestKey(arena, right))
	}
}

func TestSparseProjectionTraceAnnotationsAreACIAndPermutationCanonical(t *testing.T) {
	arena, requirements, owner := sparseProjectionACIRequirements(t)
	param := arena.Root(Root{Kind: RootParam})
	truthy, falsy := arena.Truthy(param), arena.Falsy(param)
	makeTrace := func(guard Guard) *sparseProjectionTrace {
		trace := &sparseProjectionTrace{slots: make([]sparseProjectionSlot, len(requirements))}
		trace.schema[0], trace.inventory[0] = 7, 9
		for index, requirement := range requirements {
			slot := sparseProjectionSlot{requirement: requirement, guard: guard}
			switch requirement.Stage() {
			case operationplan.RequirementBoundary:
				slot.fragments = []sparseProjectionFragment{{guard: guard, operations: []Operation{{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param}}}}
			case operationplan.RequirementObservation:
				anchor, _ := requirement.Anchor()
				kind, _ := requirement.ObservationKind()
				slot.observed = []ObservationTerm{{BodyOwner: owner, Kind: ObservationKind(kind), Anchor: anchor, Guard: guard, Actual: param, Slot: anchor.Slot}}
				slot.owed = []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: guard}}
			}
			trace.slots[index] = slot
		}
		return trace
	}
	left, right := makeTrace(truthy), makeTrace(falsy)
	lr, lrOK := joinSparseProjectionTrace(arena, left, right)
	rl, rlOK := joinSparseProjectionTrace(arena, right, left)
	if !lrOK || !rlOK || sparseProjectionTraceTestKey(arena, lr) != sparseProjectionTraceTestKey(arena, rl) {
		t.Fatalf("trace join is not commutative:\n%s\n%s", sparseProjectionTraceTestKey(arena, lr), sparseProjectionTraceTestKey(arena, rl))
	}
	idempotent, ok := joinSparseProjectionTrace(arena, lr, left)
	if !ok || sparseProjectionTraceTestKey(arena, idempotent) != sparseProjectionTraceTestKey(arena, lr) {
		t.Fatal("trace join is not idempotent")
	}
	permuted := cloneSparseProjectionTrace(lr)
	for index := range permuted.slots {
		reverseObservationTerms(permuted.slots[index].observed)
		reverseObservationObligations(permuted.slots[index].owed)
		for left, right := 0, len(permuted.slots[index].fragments)-1; left < right; left, right = left+1, right-1 {
			permuted.slots[index].fragments[left], permuted.slots[index].fragments[right] = permuted.slots[index].fragments[right], permuted.slots[index].fragments[left]
		}
	}
	builder := &sparseProjectionTraceBuilder{arena: arena, trace: *permuted}
	canonical, err := builder.freeze()
	if err != nil || sparseProjectionTraceTestKey(arena, canonical) != sparseProjectionTraceTestKey(arena, lr) {
		t.Fatalf("permuted trace did not canonicalize: %v\n%s\n%s", err, sparseProjectionTraceTestKey(arena, canonical), sparseProjectionTraceTestKey(arena, lr))
	}
}

func TestSparseProjectionTraceCancellationDoesNotPublish(t *testing.T) {
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewStringLiteralValueSource("trace", 0, 0, 0, shape)
	lowered := wir.NewBody("canceled-sparse-projection")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 39
	plan := operationplan.New(graph, factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})}}).WithObservationIdentity(owner, lowered, graph)
	reg := standard.Registry()
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	relation := prepared.evaluate(ctx, RelationView{}, nil)
	if !strings.Contains(relation.ContextualReason(), "canceled") || relation.projectionTrace != nil {
		t.Fatalf("canceled relation reason/trace = %q/%#v", relation.ContextualReason(), relation.projectionTrace)
	}
}

func TestPreparedCompilerPublishesTraceOnlyAfterExactRelationAdmission(t *testing.T) {
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewStringLiteralValueSource("admitted", 0, 0, 0, shape)
	lowered := wir.NewBody("admitted-sparse-projection")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 43
	plan := operationplan.New(graph, factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})}}).WithObservationIdentity(owner, lowered, graph)
	prepared, err := NewPlanCompiler().Prepare(standard.Registry(), graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	relation := prepared.Evaluate()
	if relation.ContextualReason() != "" || relation.projectionTrace == nil || relation.projectionTraceReason != "" {
		t.Fatalf("admitted relation reason/trace rejection = %q/%#v/%q", relation.ContextualReason(), relation.projectionTrace, relation.projectionTraceReason)
	}
	requirements, _ := plan.ObservationRequirements()
	if relation.projectionTrace.schema != requirements.SchemaID() || relation.projectionTrace.inventory != requirements.ConsumerInventoryID() || len(relation.projectionTrace.slots) != len(requirements.Entries(false)) {
		t.Fatal("published trace does not own the sealed requirement identity")
	}
}

func TestProjectionTraceMetadataParticipatesInPublicationLattice(t *testing.T) {
	reg := standard.Registry()
	builder, certificate := emptyBuilder(t, reg, Shape{}, nil)
	left := mustTestRelation(t, builder, certificate, "left")
	right := mustTestRelation(t, builder, certificate, "right")
	baselineJoin := JoinRelation(left, right)
	baselineWiden := WidenRelation(left, right)
	tracedLeft, tracedRight := left, right
	tracedLeft.projectionTrace = &sparseProjectionTrace{}
	tracedLeft.projectionTrace.schema[0] = 1
	tracedRight.projectionTrace = &sparseProjectionTrace{}
	tracedRight.projectionTrace.schema[0] = 2

	if EqualRelation(left, tracedLeft) || LessOrEqRelation(left, tracedLeft) || LessOrEqRelation(tracedLeft, left) {
		t.Fatal("publication equality/order ignored trace metadata")
	}
	joined := JoinRelation(tracedLeft, tracedRight)
	if joined.ContextualReason() != "" || joined.Widened() || joined.projectionTrace != nil || joined.projectionTraceReason == "" || EqualRelation(joined, baselineJoin) {
		t.Fatalf("incompatible trace poisoned semantic join: reason=%q widened=%v trace=%#v rejection=%q", joined.ContextualReason(), joined.Widened(), joined.projectionTrace, joined.projectionTraceReason)
	}
	if LessOrEqRelation(tracedLeft, tracedRight) || LessOrEqRelation(tracedRight, tracedLeft) {
		t.Fatal("incompatible traces entered publication order")
	}
	widened := WidenRelation(tracedLeft, tracedRight)
	if widened.ContextualReason() != "" || widened.Widened() || EqualRelation(widened, baselineWiden) || widened.projectionTrace != nil {
		t.Fatalf("incompatible trace poisoned semantic widen: %#v", widened)
	}
	baselineExact := WidenRelation(left, right)
	traceExact := WidenRelation(tracedLeft, tracedRight)
	if baselineExact.ContextualReason() != traceExact.ContextualReason() || baselineExact.Widened() != traceExact.Widened() || baselineExact.Rows() != traceExact.Rows() {
		t.Fatal("trace metadata changed exact relation widening")
	}
}

func TestPreparedRootAssignmentSelectorRetainsExactRelationAndTrace(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	literal, _ := factflow.NewIntegerLiteralValueSource(42, 0, 0, 0, shape)
	sym := symbol.ID(7)
	localPath := pathdom.Path{Root: "answer", Symbol: sym}
	read, _ := factflow.NewPathValueSource(localPath.Key(), 0, 0, 0, shape)
	lowered := wir.NewBody("unsupported-root-trace")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 45
	plan := operationplan.New(graph, factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, sym, localPath, literal)},
		Returns:         map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{read})},
	}).WithObservationIdentity(owner, lowered, graph)
	prepared, err := NewPlanCompiler().Prepare(reg, graph, plan, Shape{})
	if err != nil {
		t.Fatal(err)
	}
	relation := prepared.Evaluate()
	if relation.ContextualReason() != "" || relation.Widened() || relation.projectionTrace == nil || relation.projectionTraceReason != "" {
		t.Fatalf("root-assignment selector lost exact trace: reason=%q widened=%v trace=%#v rejection=%q", relation.ContextualReason(), relation.Widened(), relation.projectionTrace, relation.projectionTraceReason)
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	if _, exact := relation.Specialize(cursor, nil, nil); !exact {
		t.Fatal("root-assignment trace selector weakened exact specialization")
	}
}

func BenchmarkExactWTOSparseProjectionTrace(b *testing.B) {
	graph, branch, _, _, ret, requirements := sparseProjectionReturnFixture(b)
	arena := NewArena(standard.Registry())
	param := arena.Root(Root{Kind: RootParam})
	tape, err := compileSymbolicWTOTape(graph)
	if err != nil {
		b.Fatal(err)
	}
	projectionPlan, err := compileSparseProjectionPlan(requirements)
	if err != nil {
		b.Fatal(err)
	}
	transfer := func(point cfg.Point, row SymbolicCFGRow) ([]SymbolicCFGRow, error) {
		if point == ret {
			row.Operations = append(row.Operations, Operation{Kind: OutputReturn, Descriptor: DescriptorReturn, Value: param})
		}
		return []SymbolicCFGRow{row}, nil
	}
	branchTransfer := func(point cfg.Point, row SymbolicCFGRow, cond bool) (SymbolicCFGRow, Guard, error) {
		if point != branch {
			b.Fatalf("unexpected branch %d", point)
		}
		if cond {
			return row, arena.Truthy(param), nil
		}
		return row, arena.Falsy(param), nil
	}
	options := SymbolicExactWTOOptions{SymbolicCFGOptions: SymbolicCFGOptions{Shape: Shape{Params: 1}}}
	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			exit, err := solveExactWTOCFGExpandedExitRowsWithTape(context.Background(), graph, tape, arena, SymbolicCFGRow{Guard: arena.True()}, transfer, branchTransfer, options)
			if err != nil || len(exit) != 2 {
				b.Fatalf("exit/error = %d/%v", len(exit), err)
			}
		}
	})
	b.Run("trace", func(b *testing.B) {
		b.ReportMetric(float64(len(requirements.Entries(false))), "trace_slots")
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			trace := projectionPlan.newBuilder(arena, nil)
			exit, err := solveExactWTOCFGExpandedExitRowsWithTrace(context.Background(), graph, tape, arena, SymbolicCFGRow{Guard: arena.True()}, transfer, branchTransfer, options, trace)
			if err != nil || len(exit) != 2 {
				b.Fatalf("exit/error = %d/%v", len(exit), err)
			}
			if frozen, err := trace.freeze(); err != nil || frozen == nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("trace_with_plan_compile", func(b *testing.B) {
		b.ReportMetric(float64(len(requirements.Entries(false))), "trace_slots")
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			projectionPlan, err := compileSparseProjectionPlan(requirements)
			if err != nil {
				b.Fatal(err)
			}
			trace := projectionPlan.newBuilder(arena, nil)
			exit, err := solveExactWTOCFGExpandedExitRowsWithTrace(context.Background(), graph, tape, arena, SymbolicCFGRow{Guard: arena.True()}, transfer, branchTransfer, options, trace)
			if err != nil || len(exit) != 2 {
				b.Fatalf("exit/error = %d/%v", len(exit), err)
			}
			if frozen, err := trace.freeze(); err != nil || frozen == nil {
				b.Fatal(err)
			}
		}
	})
}

type sparseProjectionTestingT interface {
	Helper()
	Fatal(...any)
}

func sparseProjectionReturnFixture(t sparseProjectionTestingT) (*cfg.CFG, cfg.Point, cfg.Point, cfg.Point, cfg.Point, operationplan.ObservationRequirements) {
	t.Helper()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeNoop)
	right := graph.AddNode(cfg.NodeNoop)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, ret, false)
	graph.AddEdge(right, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewStringLiteralValueSource("trace", 0, 0, 0, shape)
	lowered := wir.NewBody("sparse-projection-return")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 37
	plan := operationplan.New(graph, factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})}}).WithObservationIdentity(owner, lowered, graph)
	requirements, sealed := plan.ObservationRequirements()
	if !sealed {
		t.Fatal("requirements not sealed")
	}
	return graph, branch, left, right, ret, requirements
}

func sparseProjectionTraceTestKey(arena *Arena, trace *sparseProjectionTrace) string {
	if trace == nil {
		return "<nil>"
	}
	var out strings.Builder
	fmt.Fprintf(&out, "schema=%x inventory=%x\n", trace.schema, trace.inventory)
	for _, slot := range trace.slots {
		to, _ := slot.requirement.EdgeTarget()
		anchor, _ := slot.requirement.Anchor()
		fmt.Fprintf(&out, "%d:%s:%d:%d:%v:%s", slot.requirement.Stage(), slot.requirement.Projection(), slot.requirement.Point(), to, anchor, arena.canonicalGuard(slot.guard))
		for _, fragment := range slot.fragments {
			fmt.Fprintf(&out, "/fragment:%s:%x", sparseProjectionFragmentKey(arena, fragment), summary.NormalizedPayloadDigest(arena.reg, fragment.output))
		}
		for _, term := range slot.observed {
			fmt.Fprintf(&out, "/observed:%s", term.canonical(arena))
		}
		for _, obligation := range slot.owed {
			fmt.Fprintf(&out, "/owed:%x:%x:%v:%s", obligation.BodyOwner, obligation.Route, obligation.Anchor, arena.canonicalGuard(obligation.Guard))
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func sparseProjectionACIRequirements(t *testing.T) (*Arena, []operationplan.ObservationRequirement, lexicalidentity.StableLexicalBodyID) {
	t.Helper()
	_, _, _, _, _, returnRequirements := sparseProjectionReturnFixture(t)
	_, observationPlan, owner, _ := observationCoverageFixture(t)
	observationRequirements, ok := observationPlan.ObservationRequirements()
	if !ok {
		t.Fatal("observation requirements not sealed")
	}
	requirements := make([]operationplan.ObservationRequirement, 0, 5)
	for _, requirement := range returnRequirements.Entries(false) {
		if len(requirements) == 0 && requirement.Stage() == operationplan.RequirementPoint ||
			requirement.Stage() == operationplan.RequirementBoundary || requirement.Stage() == operationplan.RequirementEdge {
			requirements = append(requirements, requirement)
		}
	}
	for _, requirement := range observationRequirements.Entries(false) {
		if requirement.Stage() == operationplan.RequirementObservation {
			requirements = append(requirements, requirement)
			break
		}
	}
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	lowered := wir.NewBody("trace-route")
	start := lowered.Len()
	lowered.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	lowered.SetPointRange(call, start, lowered.Len())
	lowered.AssignDebugPointOrdinals(graph)
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: call, HasPoint: true, ArgumentSources: []factflow.ValueSource{{}}, Final: true})
	routePlan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{call: site}}).WithObservationIdentity(owner, lowered, graph)
	routeRequirements, ok := routePlan.ObservationRequirements()
	if !ok {
		t.Fatal("route requirements not sealed")
	}
	for _, requirement := range routeRequirements.Entries(false) {
		if requirement.Stage() == operationplan.RequirementRoute {
			requirements = append(requirements, requirement)
			break
		}
	}
	return NewArena(standard.Registry()), requirements, owner
}

func reverseObservationTerms(in []ObservationTerm) {
	for left, right := 0, len(in)-1; left < right; left, right = left+1, right-1 {
		in[left], in[right] = in[right], in[left]
	}
}

func reverseObservationObligations(in []observationObligation) {
	for left, right := 0, len(in)-1; left < right; left, right = left+1, right-1 {
		in[left], in[right] = in[right], in[left]
	}
}
