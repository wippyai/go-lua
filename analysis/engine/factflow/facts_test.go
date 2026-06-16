package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDTOConstructorsAndAccessorsCopySlices(t *testing.T) {
	source := ValueSource{
		Kind:        ValueSourceExpression,
		ExprRef:     ExprRef(1),
		HasExpr:     true,
		ExprIndex:   0,
		TargetIndex: 0,
		ResultIndex: NoValueSourceIndex,
	}
	callSource := ValueSource{
		Kind:         ValueSourceCall,
		ExprRef:      ExprRef(2),
		HasExpr:      true,
		ExprIndex:    1,
		TargetIndex:  1,
		ResultIndex:  0,
		CallPoint:    cfg.Point(99),
		HasCallPoint: true,
		Final:        true,
		Adjusted:     true,
	}

	localPath := path.NewPath(symbol.ID(10), "local")
	local := NewRootAssignment(RootAssignmentLocalDeclaration, symbol.ID(10), localPath, source)
	if got := local.Kind(); got != RootAssignmentLocalDeclaration {
		t.Fatalf("local kind = %v, want local declaration", got)
	}
	assertPathEqual(t, local.TargetPath(), localPath)
	if got := local.Source(); got != source {
		t.Fatalf("local source = %#v, want %#v", got, source)
	}

	ordinaryPath := path.NewPath(symbol.ID(11), "ordinary")
	ordinary := NewRootAssignment(RootAssignmentOrdinaryRootWrite, symbol.ID(11), ordinaryPath, source)
	if got := ordinary.Kind(); got != RootAssignmentOrdinaryRootWrite {
		t.Fatalf("ordinary kind = %v, want ordinary root write", got)
	}
	assertPathEqual(t, ordinary.TargetPath(), ordinaryPath)
	if got := ordinary.Source(); got != source {
		t.Fatalf("ordinary source = %#v, want %#v", got, source)
	}

	pathTarget := path.NewPath(symbol.ID(14), "table").Field("field")
	pathAssignment := NewPathAssignment(pathTarget, source)
	assertPathEqual(t, pathAssignment.TargetPath(), pathTarget)
	if got := pathAssignment.Source(); got != source {
		t.Fatalf("path assignment source = %#v, want %#v", got, source)
	}
	pathTarget.Segments[0].Name = "changed"
	assertDirectField(t, pathAssignment.TargetPath(), "field")
	gotPathTarget := pathAssignment.TargetPath()
	gotPathTarget.Segments[0].Name = "changed-again"
	assertDirectField(t, pathAssignment.TargetPath(), "field")

	invalidationTarget := path.NewPath(symbol.ID(23), "item").Field("child")
	invalidation := NewPathDescendantInvalidation(invalidationTarget)
	assertPathEqual(t, invalidation.ContainerPath(), invalidationTarget)
	invalidationTarget.Segments[0].Name = "changed"
	assertDirectField(t, invalidation.ContainerPath(), "child")
	gotInvalidationTarget := invalidation.ContainerPath()
	gotInvalidationTarget.Segments[0].Name = "changed-again"
	assertDirectField(t, invalidation.ContainerPath(), "child")

	branchTarget := path.NewPath(symbol.ID(15), "value").Field("ready")
	trueRefinement := valueRefinementWithPresenceRuntime(presence.Present(), runtimekind.Singleton(runtimekind.Table))
	falseRefinement := valueRefinementWithPresenceRuntime(presence.Absent(), runtimekind.Singleton(runtimekind.Nil))
	branchRefinement := NewBranchRefinement(branchTarget, trueRefinement, true, falseRefinement, true)
	assertPathEqual(t, branchRefinement.TargetPath(), branchTarget)
	if got, ok := branchRefinement.TrueValue(); !ok || got.IsEmpty() {
		t.Fatalf("true value refinement = %#v/%v, want non-empty/true", got, ok)
	}
	if got, ok := branchRefinement.FalseValue(); !ok || got.IsEmpty() {
		t.Fatalf("false value refinement = %#v/%v, want non-empty/true", got, ok)
	}
	gotTrue, _ := branchRefinement.TrueValue()
	assertValueRefinementConstraint(t, "true", gotTrue, presence.Present(), runtimekind.Singleton(runtimekind.Table))
	gotFalse, _ := branchRefinement.FalseValue()
	assertValueRefinementConstraint(t, "false", gotFalse, presence.Absent(), runtimekind.Singleton(runtimekind.Nil))
	if got, ok := branchRefinement.ValueForEdge(true); !ok || got.IsEmpty() {
		t.Fatalf("true edge value = %#v/%v, want non-empty/true", got, ok)
	}
	branchTarget.Segments[0].Name = "changed"
	assertDirectField(t, branchRefinement.TargetPath(), "ready")
	gotBranchTarget := branchRefinement.TargetPath()
	gotBranchTarget.Segments[0].Name = "changed-again"
	assertDirectField(t, branchRefinement.TargetPath(), "ready")
	branchSet := NewBranchRefinementSet(branchRefinement)
	branchSetRefinements := branchSet.Refinements()
	if len(branchSetRefinements) != 1 {
		t.Fatalf("branch set refinements len = %d, want 1", len(branchSetRefinements))
	}
	branchSetRefinements[0] = NewBranchRefinement(path.NewPath(symbol.ID(18), "mutated"), trueRefinement, true, falseRefinement, true)
	assertPathEqual(t, branchSet.Refinements()[0].TargetPath(), path.NewPath(symbol.ID(15), "value").Field("ready"))

	relationTrigger := path.NewPath(symbol.ID(19), "err")
	relationTarget := path.NewPath(symbol.ID(20), "value")
	relation := NewBranchPresenceRelation(relationTrigger, presence.Present(), relationTarget, presence.Absent())
	assertPathEqual(t, relation.TriggerPath(), relationTrigger)
	assertPathEqual(t, relation.TargetPath(), relationTarget)
	if !presence.Equal(relation.TriggerPresence(), presence.Present()) || !presence.Equal(relation.TargetPresence(), presence.Absent()) {
		t.Fatalf("relation presence = %s/%s, want present/absent", relation.TriggerPresence(), relation.TargetPresence())
	}
	relationTrigger.Root = "changed"
	relationTarget.Root = "changed"
	assertPathEqual(t, relation.TriggerPath(), path.NewPath(symbol.ID(19), "err"))
	assertPathEqual(t, relation.TargetPath(), path.NewPath(symbol.ID(20), "value"))
	relationSet := NewBranchPresenceRelationSet(relation)
	relationSetRelations := relationSet.Relations()
	if len(relationSetRelations) != 1 {
		t.Fatalf("relation set len = %d, want 1", len(relationSetRelations))
	}
	relationSetRelations[0] = NewBranchPresenceRelation(path.NewPath(symbol.ID(21), "other"), presence.Absent(), path.NewPath(symbol.ID(22), "changed"), presence.Present())
	assertPathEqual(t, relationSet.Relations()[0].TriggerPath(), path.NewPath(symbol.ID(19), "err"))

	pathRelationLeft := path.NewPath(symbol.ID(24), "left").Field("value")
	pathRelationRight := path.NewPath(symbol.ID(25), "right").Field("value")
	pathRelation := NewBranchPathEquality(pathRelationLeft, pathRelationRight, true, false)
	if pathRelation.Kind() != BranchPathRelationEqual {
		t.Fatalf("path relation kind = %v, want equality", pathRelation.Kind())
	}
	assertPathEqual(t, pathRelation.LeftPath(), pathRelationLeft)
	assertPathEqual(t, pathRelation.RightPath(), pathRelationRight)
	if !pathRelation.ActiveOnEdge(true) || pathRelation.ActiveOnEdge(false) {
		t.Fatalf("path relation active true/false = %v/%v, want true/false", pathRelation.ActiveOnEdge(true), pathRelation.ActiveOnEdge(false))
	}
	pathRelationLeft.Segments[0].Name = "changed"
	pathRelationRight.Segments[0].Name = "changed"
	assertPathEqual(t, pathRelation.LeftPath(), path.NewPath(symbol.ID(24), "left").Field("value"))
	assertPathEqual(t, pathRelation.RightPath(), path.NewPath(symbol.ID(25), "right").Field("value"))
	pathRelationSet := NewBranchPathRelationSet(pathRelation)
	pathRelationSetRelations := pathRelationSet.Relations()
	if len(pathRelationSetRelations) != 1 {
		t.Fatalf("path relation set len = %d, want 1", len(pathRelationSetRelations))
	}
	pathRelationSetRelations[0] = NewBranchPathEquality(path.NewPath(symbol.ID(26), "mutated"), path.NewPath(symbol.ID(27), "mutated"), false, true)
	assertPathEqual(t, pathRelationSet.Relations()[0].LeftPath(), path.NewPath(symbol.ID(24), "left").Field("value"))

	postconditionTarget := path.NewPath(symbol.ID(28), "post").Field("value")
	postcondition := NewPostconditionRefinement(postconditionTarget, trueRefinement)
	assertPathEqual(t, postcondition.TargetPath(), postconditionTarget)
	if got := postcondition.Value(); got.IsEmpty() {
		t.Fatalf("postcondition value = %#v, want non-empty", got)
	}
	postconditionTarget.Segments[0].Name = "changed"
	assertDirectField(t, postcondition.TargetPath(), "value")
	gotPostconditionTarget := postcondition.TargetPath()
	gotPostconditionTarget.Segments[0].Name = "changed-again"
	assertDirectField(t, postcondition.TargetPath(), "value")
	postconditionSet := NewPostconditionRefinementSet(postcondition)
	postconditionSetRefinements := postconditionSet.Refinements()
	if len(postconditionSetRefinements) != 1 {
		t.Fatalf("postcondition set len = %d, want 1", len(postconditionSetRefinements))
	}
	postconditionSetRefinements[0] = NewPostconditionRefinement(path.NewPath(symbol.ID(29), "mutated"), falseRefinement)
	assertPathEqual(t, postconditionSet.Refinements()[0].TargetPath(), path.NewPath(symbol.ID(28), "post").Field("value"))

	entrySuffix := path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}
	entry := NewObjectEntry(entrySuffix, source)
	entrySuffix.Segments[0].Name = "changed"
	if got := entry.Suffix(); len(got.Segments) != 1 || got.Segments[0].Name != "field" {
		t.Fatalf("object entry suffix = %#v, want copied field suffix", got)
	}
	gotEntrySuffix := entry.Suffix()
	gotEntrySuffix.Segments[0].Name = "changed-again"
	if got := entry.Suffix(); got.Segments[0].Name != "field" {
		t.Fatalf("object entry exposed mutable suffix: %#v", got)
	}
	if got := entry.Source(); got != source {
		t.Fatalf("object entry source = %#v, want %#v", got, source)
	}

	entries := []ObjectEntry{entry}
	literal := NewObjectLiteral(entries)
	entries[0] = NewObjectEntry(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "other"}}}, callSource)
	if got := literal.Entries(); len(got) != 1 || got[0].Source() != source {
		t.Fatalf("object literal entries = %#v, want copied entry", got)
	}
	gotEntries := literal.Entries()
	gotEntries[0] = NewObjectEntry(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "mutated"}}}, callSource)
	if got := literal.Entries(); got[0].Source() != source {
		t.Fatalf("object literal exposed mutable entries: %#v", got)
	}

	refinementValue := runtimeKindConstraint(runtimekind.Singleton(runtimekind.Table))
	refinement := NewExpressionRefinement(source, refinementValue)
	if got := refinement.Source(); got != source {
		t.Fatalf("expression refinement source = %#v, want %#v", got, source)
	}
	if got := refinement.Refinement(); !product.Equal(standard.Registry(), got, refinementValue) {
		t.Fatalf("expression refinement = %s, want %s", formatValue(standard.Registry(), got), formatValue(standard.Registry(), refinementValue))
	}

	returnSources := []ValueSource{source, callSource}
	ret := NewReturn(returnSources)
	returnSources[0].Kind = ValueSourceNil
	if got := ret.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("return source copied from input as %v, want %v", got[0].Kind, ValueSourceExpression)
	}
	gotReturnSources := ret.Sources()
	gotReturnSources[0].Kind = ValueSourceNil
	if got := ret.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("return source exposed mutable slice, got %v", got[0].Kind)
	}

	calleePath := path.NewPath(symbol.ID(12), "callee").Field("method")
	targetPath := path.NewPath(symbol.ID(13), "target")
	target := NewCallResultTarget(CallResultTargetLocalAssignment, 0, 0, symbol.ID(13), targetPath)
	assertPathEqual(t, target.TargetPath(), targetPath)
	if target.ResultIndex() != 0 {
		t.Fatalf("call result target slot = %d, want 0", target.ResultIndex())
	}

	targets := []CallResultTarget{target}
	call := NewCallProducer(CallProducerConfig{
		CalleeSymbol:  symbol.ID(12),
		CalleePath:    calleePath,
		ResultTargets: targets,
	})
	calleePath.Segments[0].Name = "changed"
	targets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, 0, path.Path{})

	assertDirectField(t, call.CalleePath(), "method")
	gotCalleePath := call.CalleePath()
	gotCalleePath.Segments[0].Name = "changed-again"
	assertDirectField(t, call.CalleePath(), "method")
	if call.CalleeSymbol() != symbol.ID(12) {
		t.Fatalf("call symbol = %v", call.CalleeSymbol())
	}
	gotTargets := call.ResultTargets()
	if len(gotTargets) != 1 || gotTargets[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("call targets = %#v, want one local-assignment target", gotTargets)
	}
	gotTargets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, 0, path.Path{})
	if got := call.ResultTargets(); got[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("call result targets exposed mutable slice, got %v", got[0].Kind())
	}
	gotCallTargetPath := call.ResultTargets()[0].TargetPath()
	assertPathEqual(t, gotCallTargetPath, targetPath)
	assertPathEqual(t, call.ResultTargets()[0].TargetPath(), targetPath)

	siteCalleePath := path.NewPath(symbol.ID(16), "svc").Field("run")
	siteReceiverPath := path.NewPath(symbol.ID(16), "svc")
	siteMethodPath := siteReceiverPath.Field("run")
	siteTargetPath := path.NewPath(symbol.ID(17), "t").Field("x")
	siteArgs := []ValueSource{source, callSource}
	siteTypeArgs := []TypeRef{TypeRef(1), TypeRef(2)}
	siteTargets := []CallResultTarget{
		NewCallResultTarget(CallResultTargetOrdinaryAssignment, 0, 0, symbol.ID(17), siteTargetPath),
	}
	site := NewCallSite(CallSiteConfig{
		Context:         CallSiteContextCondition,
		CalleeSymbol:    symbol.ID(16),
		CalleePath:      siteCalleePath,
		ReceiverPath:    siteReceiverPath,
		HasReceiverPath: true,
		MethodPath:      siteMethodPath,
		HasMethodPath:   true,
		MethodName:      "run",
		ExprRef:         ExprRef(4),
		HasExpr:         true,
		ExprIndex:       0,
		ArgumentSources: siteArgs,
		TypeArgs:        siteTypeArgs,
		ResultTargets:   siteTargets,
		Final:           true,
		Adjusted:        true,
	})
	siteCalleePath.Segments[0].Name = "changed"
	siteReceiverPath.Root = "changed"
	siteMethodPath.Segments[0].Name = "changed"
	siteArgs[0].Kind = ValueSourceNil
	siteTypeArgs[0] = TypeRef(99)
	siteTargets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, 0, path.Path{})
	assertDirectField(t, site.CalleePath(), "run")
	gotSiteCalleePath := site.CalleePath()
	gotSiteCalleePath.Segments[0].Name = "changed-again"
	assertDirectField(t, site.CalleePath(), "run")
	if receiverPath, ok := site.ReceiverPath(); !ok || !receiverPath.Equal(path.NewPath(symbol.ID(16), "svc")) {
		t.Fatalf("call site receiver path = %#v/%v", receiverPath, ok)
	}
	gotReceiverPath, _ := site.ReceiverPath()
	gotReceiverPath.Root = "changed-again"
	if receiverPath, _ := site.ReceiverPath(); !receiverPath.Equal(path.NewPath(symbol.ID(16), "svc")) {
		t.Fatalf("call site receiver path exposed mutable path: %#v", receiverPath)
	}
	if methodPath, ok := site.MethodPath(); !ok || !methodPath.Equal(path.NewPath(symbol.ID(16), "svc").Field("run")) {
		t.Fatalf("call site method path = %#v/%v", methodPath, ok)
	}
	gotMethodPath, _ := site.MethodPath()
	gotMethodPath.Segments[0].Name = "changed-again"
	methodPathAgain, _ := site.MethodPath()
	assertDirectField(t, methodPathAgain, "run")
	if site.MethodName() != "run" {
		t.Fatalf("call site method name = %q, want run", site.MethodName())
	}
	if site.Context() != CallSiteContextCondition || site.CalleeSymbol() != symbol.ID(16) || site.ExprIndex() != 0 {
		t.Fatalf("call site context/symbol/expr index = %v/%v/%v", site.Context(), site.CalleeSymbol(), site.ExprIndex())
	}
	if expr, ok := site.Expr(); !ok || expr != ExprRef(4) {
		t.Fatalf("call site expr = %v/%v, want %v/true", expr, ok, ExprRef(4))
	}
	if !site.Final() || site.Expanded() || !site.Adjusted() || site.OpenTail() {
		t.Fatalf("call site flags were not preserved")
	}
	gotSiteArgs := site.ArgumentSources()
	if len(gotSiteArgs) != 2 || gotSiteArgs[0].Kind != ValueSourceExpression || gotSiteArgs[1].Kind != ValueSourceCall {
		t.Fatalf("call site args = %#v, want copied argument sources", gotSiteArgs)
	}
	gotSiteArgs[0].Kind = ValueSourceNil
	if got := site.ArgumentSources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("call site exposed mutable argument sources, got %v", got[0].Kind)
	}
	gotSiteTypeArgs := site.TypeArgs()
	if len(gotSiteTypeArgs) != 2 || gotSiteTypeArgs[0] != TypeRef(1) || gotSiteTypeArgs[1] != TypeRef(2) {
		t.Fatalf("call site type args = %#v, want copied type refs", gotSiteTypeArgs)
	}
	gotSiteTypeArgs[0] = TypeRef(99)
	if got := site.TypeArgs(); got[0] != TypeRef(1) {
		t.Fatalf("call site exposed mutable type args, got %#v", got)
	}
	gotSiteTargets := site.ResultTargets()
	if len(gotSiteTargets) != 1 || gotSiteTargets[0].Kind() != CallResultTargetOrdinaryAssignment {
		t.Fatalf("call site targets = %#v, want one ordinary-assignment target", gotSiteTargets)
	}
	assertDirectField(t, gotSiteTargets[0].TargetPath(), "x")
	gotSiteTargets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, 0, path.Path{})
	if got := site.ResultTargets(); got[0].Kind() != CallResultTargetOrdinaryAssignment {
		t.Fatalf("call site result targets exposed mutable slice, got %v", got[0].Kind())
	}
}

func TestTransferOwnedEnumsAreIndependentContracts(t *testing.T) {
	kinds := []ValueSourceKind{
		ValueSourceUnknown,
		ValueSourceExpression,
		ValueSourceCall,
		ValueSourceVararg,
		ValueSourceNil,
	}
	if len(kinds) != 5 || kinds[0] != ValueSourceUnknown || kinds[4] != ValueSourceNil {
		t.Fatalf("unexpected value source kinds: %#v", kinds)
	}

	siteContexts := []CallSiteContext{
		CallSiteContextUnknown,
		CallSiteContextStatement,
		CallSiteContextAssignmentSource,
		CallSiteContextReturnSource,
		CallSiteContextIteratorSource,
		CallSiteContextCondition,
		CallSiteContextExpressionProducer,
	}
	if len(siteContexts) != 7 || siteContexts[6] != CallSiteContextExpressionProducer {
		t.Fatalf("unexpected call site contexts: %#v", siteContexts)
	}

	targets := []CallResultTargetKind{
		CallResultTargetUnknown,
		CallResultTargetLocalAssignment,
		CallResultTargetOrdinaryAssignment,
		CallResultTargetReturn,
		CallResultTargetExpression,
	}
	if len(targets) != 5 || targets[4] != CallResultTargetExpression {
		t.Fatalf("unexpected call result target kinds: %#v", targets)
	}
}

func TestFactsCarrierCopiesAndReturnsFalseForMissingFacts(t *testing.T) {
	localPoint := cfg.Point(20)
	ordinaryPoint := cfg.Point(21)
	point := localPoint
	missing := cfg.Point(22)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(1), HasExpr: true}
	callSource := ValueSource{Kind: ValueSourceCall, ExprRef: ExprRef(2), HasExpr: true}

	input := FactsInput{
		RootAssignments: map[cfg.Point]RootAssignment{
			localPoint:    NewRootAssignment(RootAssignmentLocalDeclaration, symbol.ID(30), path.NewPath(symbol.ID(30), "local"), source),
			ordinaryPoint: NewRootAssignment(RootAssignmentOrdinaryRootWrite, symbol.ID(31), path.NewPath(symbol.ID(31), "ordinary"), source),
		},
		PathAssignments: map[cfg.Point]PathAssignment{
			point: NewPathAssignment(path.NewPath(symbol.ID(33), "table").Field("field"), source),
		},
		PathDescendantInvalidations: map[cfg.Point]PathDescendantInvalidation{
			point: NewPathDescendantInvalidation(path.NewPath(symbol.ID(36), "item").Field("container")),
		},
		BranchRefinements: map[cfg.Point]BranchRefinementSet{
			point: NewBranchRefinementSet(
				NewBranchRefinement(
					path.NewPath(symbol.ID(34), "value").Field("ready"),
					valueRefinementWithPresenceRuntime(presence.Present(), runtimekind.Singleton(runtimekind.Table)), true,
					valueRefinementWithPresenceRuntime(presence.Absent(), runtimekind.Singleton(runtimekind.Nil)), true,
				),
				NewBranchRefinement(
					path.NewPath(symbol.ID(44), "other").Field("ready"),
					valueRefinementWithPresence(presence.Present()), true,
					valueRefinementWithPresence(presence.Absent()), true,
				),
			),
		},
		BranchPresenceRelations: map[cfg.Point]BranchPresenceRelationSet{
			point: NewBranchPresenceRelationSet(
				NewBranchPresenceRelation(
					path.NewPath(symbol.ID(47), "err"),
					presence.Present(),
					path.NewPath(symbol.ID(48), "value"),
					presence.Absent(),
				),
			),
		},
		BranchPathRelations: map[cfg.Point]BranchPathRelationSet{
			point: NewBranchPathRelationSet(
				NewBranchPathEquality(
					path.NewPath(symbol.ID(57), "left").Field("value"),
					path.NewPath(symbol.ID(58), "right").Field("value"),
					true,
					false,
				),
			),
		},
		PostconditionRefinements: map[cfg.Point]PostconditionRefinementSet{
			point: NewPostconditionRefinementSet(
				NewPostconditionRefinement(
					path.NewPath(symbol.ID(63), "post").Field("value"),
					valueRefinementWithPresenceRuntime(presence.Present(), runtimekind.Singleton(runtimekind.String)),
				),
			),
		},
		Returns: map[cfg.Point]Return{
			point: NewReturn([]ValueSource{source, callSource}),
		},
		CallSites: map[cfg.Point]CallSite{
			point: NewCallSite(CallSiteConfig{
				Context:         CallSiteContextAssignmentSource,
				CalleeSymbol:    symbol.ID(35),
				CalleePath:      path.NewPath(symbol.ID(35), "callee").Field("site"),
				ReceiverPath:    path.NewPath(symbol.ID(35), "callee"),
				HasReceiverPath: true,
				MethodPath:      path.NewPath(symbol.ID(35), "callee").Field("site"),
				HasMethodPath:   true,
				MethodName:      "site",
				ExprRef:         ExprRef(5),
				HasExpr:         true,
				ExprIndex:       1,
				ArgumentSources: []ValueSource{source, callSource},
				TypeArgs:        []TypeRef{TypeRef(7), TypeRef(8)},
				ResultTargets: []CallResultTarget{
					NewCallResultTarget(CallResultTargetLocalAssignment, 0, 0, symbol.ID(33), path.NewPath(symbol.ID(33), "table")),
				},
				Final:    true,
				Expanded: true,
			}),
		},
		ObjectLiterals: map[ExprRef]ObjectLiteral{
			ExprRef(1): NewObjectLiteral([]ObjectEntry{
				NewObjectEntry(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "field"}}}, source),
			}),
		},
		ExpressionValues: map[ExprRef]product.Value{
			ExprRef(3): product.Set(standard.Registry(), product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String)),
		},
		ExpressionRefinements: map[ExprRef]ExpressionRefinement{
			ExprRef(4): NewExpressionRefinement(source, runtimeKindConstraint(runtimekind.Singleton(runtimekind.Table))),
		},
		ExpressionPaths: map[ExprRef]path.Path{
			ExprRef(6): path.NewPath(symbol.ID(54), "read").Field("leaf"),
		},
	}

	facts := NewFacts(input)
	input.RootAssignments[localPoint] = NewRootAssignment(RootAssignmentLocalDeclaration, symbol.ID(40), path.NewPath(symbol.ID(40), "changed"), callSource)
	input.RootAssignments[ordinaryPoint] = NewRootAssignment(RootAssignmentOrdinaryRootWrite, symbol.ID(41), path.NewPath(symbol.ID(41), "changed"), callSource)
	input.PathAssignments[point] = NewPathAssignment(path.NewPath(symbol.ID(42), "changed").Field("field"), callSource)
	input.PathDescendantInvalidations[point] = NewPathDescendantInvalidation(path.NewPath(symbol.ID(53), "changed"))
	input.BranchRefinements[point] = NewBranchRefinementSet(
		NewBranchRefinement(
			path.NewPath(symbol.ID(43), "changed").Field("field"),
			valueRefinementWithPresence(presence.Absent()), true,
			valueRefinementWithPresence(presence.Present()), true,
		),
		NewBranchRefinement(
			path.NewPath(symbol.ID(45), "changed").Field("field"),
			valueRefinementWithPresence(presence.Absent()), true,
			valueRefinementWithPresence(presence.Present()), true,
		),
	)
	input.BranchPresenceRelations[point] = NewBranchPresenceRelationSet(
		NewBranchPresenceRelation(
			path.NewPath(symbol.ID(49), "changed"),
			presence.Absent(),
			path.NewPath(symbol.ID(50), "changed"),
			presence.Present(),
		),
	)
	input.BranchPathRelations[point] = NewBranchPathRelationSet(
		NewBranchPathEquality(
			path.NewPath(symbol.ID(59), "changed"),
			path.NewPath(symbol.ID(60), "changed"),
			false,
			true,
		),
	)
	input.PostconditionRefinements[point] = NewPostconditionRefinementSet(
		NewPostconditionRefinement(
			path.NewPath(symbol.ID(64), "changed"),
			valueRefinementWithPresence(presence.Absent()),
		),
	)
	input.Returns[point] = NewReturn([]ValueSource{{Kind: ValueSourceNil}})
	input.CallSites[point] = NewCallSite(CallSiteConfig{Context: CallSiteContextStatement})
	input.ObjectLiterals[ExprRef(1)] = NewObjectLiteral([]ObjectEntry{
		NewObjectEntry(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "changed"}}}, callSource),
	})
	input.ExpressionValues[ExprRef(3)] = product.Set(standard.Registry(), product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	input.ExpressionRefinements[ExprRef(4)] = NewExpressionRefinement(callSource, runtimeKindConstraint(runtimekind.Singleton(runtimekind.Function)))
	input.ExpressionPaths[ExprRef(6)] = path.NewPath(symbol.ID(55), "changed").Field("leaf")

	if _, ok := facts.LocalAssignment(missing); ok {
		t.Fatal("missing local assignment returned ok")
	}
	if _, ok := facts.OrdinaryAssignment(missing); ok {
		t.Fatal("missing ordinary assignment returned ok")
	}
	if _, ok := facts.PathAssignment(missing); ok {
		t.Fatal("missing path assignment returned ok")
	}
	if _, ok := facts.PathDescendantInvalidation(missing); ok {
		t.Fatal("missing path descendant invalidation returned ok")
	}
	if got := facts.BranchRefinements(missing); len(got) != 0 {
		t.Fatalf("missing branch refinements = %#v, want empty", got)
	}
	if got := facts.BranchPresenceRelations(missing); len(got) != 0 {
		t.Fatalf("missing branch presence relations = %#v, want empty", got)
	}
	if got := facts.BranchPathRelations(missing); len(got) != 0 {
		t.Fatalf("missing branch path relations = %#v, want empty", got)
	}
	if got := facts.PostconditionRefinements(missing); len(got) != 0 {
		t.Fatalf("missing postcondition refinements = %#v, want empty", got)
	}
	if _, ok := facts.Return(missing); ok {
		t.Fatal("missing return returned ok")
	}
	if _, ok := facts.CallSite(missing); ok {
		t.Fatal("missing call site returned ok")
	}
	if _, ok := facts.ObjectLiteral(ExprRef(99)); ok {
		t.Fatal("missing object literal returned ok")
	}
	if _, ok := facts.ExpressionValue(ExprRef(99)); ok {
		t.Fatal("missing expression value returned ok")
	}
	if _, ok := facts.ExpressionRefinement(ExprRef(99)); ok {
		t.Fatal("missing expression refinement returned ok")
	}
	if _, ok := facts.ExpressionPath(ExprRef(99)); ok {
		t.Fatal("missing expression path returned ok")
	}

	local, ok := facts.LocalAssignment(localPoint)
	if !ok {
		t.Fatal("local assignment missing")
	}
	if local.TargetSymbol() != symbol.ID(30) {
		t.Fatalf("local target symbol = %v, want 30", local.TargetSymbol())
	}
	localAgain, _ := facts.LocalAssignment(localPoint)
	assertPathEqual(t, localAgain.TargetPath(), path.NewPath(symbol.ID(30), "local"))

	root, ok := facts.RootAssignment(ordinaryPoint)
	if !ok {
		t.Fatal("root assignment missing")
	}
	if root.Kind() != RootAssignmentOrdinaryRootWrite {
		t.Fatalf("root assignment kind = %v, want ordinary root write", root.Kind())
	}

	ordinary, ok := facts.OrdinaryAssignment(ordinaryPoint)
	if !ok {
		t.Fatal("ordinary assignment missing")
	}
	if ordinary.TargetSymbol() != symbol.ID(31) {
		t.Fatalf("ordinary target symbol = %v, want 31", ordinary.TargetSymbol())
	}
	ordinaryAgain, _ := facts.OrdinaryAssignment(ordinaryPoint)
	assertPathEqual(t, ordinaryAgain.TargetPath(), path.NewPath(symbol.ID(31), "ordinary"))

	pathAssignment, ok := facts.PathAssignment(point)
	if !ok {
		t.Fatal("path assignment missing")
	}
	assertPathEqual(t, pathAssignment.TargetPath(), path.NewPath(symbol.ID(33), "table").Field("field"))
	pathAssignmentPath := pathAssignment.TargetPath()
	pathAssignmentPath.Segments[0].Name = "mutated"
	pathAssignmentAgain, _ := facts.PathAssignment(point)
	assertDirectField(t, pathAssignmentAgain.TargetPath(), "field")

	invalidationFact, ok := facts.PathDescendantInvalidation(point)
	if !ok {
		t.Fatal("path descendant invalidation missing")
	}
	assertPathEqual(t, invalidationFact.ContainerPath(), path.NewPath(symbol.ID(36), "item").Field("container"))
	invalidationPath := invalidationFact.ContainerPath()
	invalidationPath.Segments[0].Name = "mutated"
	invalidationAgain, _ := facts.PathDescendantInvalidation(point)
	assertDirectField(t, invalidationAgain.ContainerPath(), "container")

	branchRefinements := facts.BranchRefinements(point)
	if len(branchRefinements) != 2 {
		t.Fatalf("branch refinements len = %d, want 2", len(branchRefinements))
	}
	branchRefinement := branchRefinements[0]
	assertPathEqual(t, branchRefinement.TargetPath(), path.NewPath(symbol.ID(34), "value").Field("ready"))
	branchRefinementPath := branchRefinement.TargetPath()
	branchRefinementPath.Segments[0].Name = "mutated"
	branchRefinementAgain := facts.BranchRefinements(point)[0]
	assertDirectField(t, branchRefinementAgain.TargetPath(), "ready")
	trueValue, ok := branchRefinementAgain.TrueValue()
	if !ok {
		t.Fatalf("branch true value missing")
	}
	assertValueRefinementConstraint(t, "branch true", trueValue, presence.Present(), runtimekind.Singleton(runtimekind.Table))
	falseValue, ok := branchRefinementAgain.FalseValue()
	if !ok {
		t.Fatalf("branch false value missing")
	}
	assertValueRefinementConstraint(t, "branch false", falseValue, presence.Absent(), runtimekind.Singleton(runtimekind.Nil))
	assertPathEqual(t, branchRefinements[1].TargetPath(), path.NewPath(symbol.ID(44), "other").Field("ready"))
	branchRefinements[1] = NewBranchRefinement(
		path.NewPath(symbol.ID(46), "mutated"),
		valueRefinementWithPresence(presence.Absent()), true,
		ValueRefinement{}, false,
	)
	branchRefinementsAgain := facts.BranchRefinements(point)
	assertPathEqual(t, branchRefinementsAgain[1].TargetPath(), path.NewPath(symbol.ID(44), "other").Field("ready"))

	relations := facts.BranchPresenceRelations(point)
	if len(relations) != 1 {
		t.Fatalf("branch presence relations len = %d, want 1", len(relations))
	}
	assertPathEqual(t, relations[0].TriggerPath(), path.NewPath(symbol.ID(47), "err"))
	assertPathEqual(t, relations[0].TargetPath(), path.NewPath(symbol.ID(48), "value"))
	if !presence.Equal(relations[0].TriggerPresence(), presence.Present()) || !presence.Equal(relations[0].TargetPresence(), presence.Absent()) {
		t.Fatalf("branch presence relation presence = %s/%s, want present/absent", relations[0].TriggerPresence(), relations[0].TargetPresence())
	}
	relations[0] = NewBranchPresenceRelation(path.NewPath(symbol.ID(51), "mutated"), presence.Absent(), path.NewPath(symbol.ID(52), "mutated"), presence.Present())
	relationsAgain := facts.BranchPresenceRelations(point)
	assertPathEqual(t, relationsAgain[0].TriggerPath(), path.NewPath(symbol.ID(47), "err"))

	pathRelations := facts.BranchPathRelations(point)
	if len(pathRelations) != 1 {
		t.Fatalf("branch path relations len = %d, want 1", len(pathRelations))
	}
	if pathRelations[0].Kind() != BranchPathRelationEqual {
		t.Fatalf("branch path relation kind = %v, want equality", pathRelations[0].Kind())
	}
	assertPathEqual(t, pathRelations[0].LeftPath(), path.NewPath(symbol.ID(57), "left").Field("value"))
	assertPathEqual(t, pathRelations[0].RightPath(), path.NewPath(symbol.ID(58), "right").Field("value"))
	if !pathRelations[0].ActiveOnEdge(true) || pathRelations[0].ActiveOnEdge(false) {
		t.Fatalf("branch path relation active true/false = %v/%v, want true/false", pathRelations[0].ActiveOnEdge(true), pathRelations[0].ActiveOnEdge(false))
	}
	pathRelations[0] = NewBranchPathEquality(path.NewPath(symbol.ID(61), "mutated"), path.NewPath(symbol.ID(62), "mutated"), false, true)
	pathRelationsAgain := facts.BranchPathRelations(point)
	assertPathEqual(t, pathRelationsAgain[0].LeftPath(), path.NewPath(symbol.ID(57), "left").Field("value"))

	postconditions := facts.PostconditionRefinements(point)
	if len(postconditions) != 1 {
		t.Fatalf("postcondition refinements len = %d, want 1", len(postconditions))
	}
	assertPathEqual(t, postconditions[0].TargetPath(), path.NewPath(symbol.ID(63), "post").Field("value"))
	assertValueRefinementConstraint(t, "postcondition", postconditions[0].Value(), presence.Present(), runtimekind.Singleton(runtimekind.String))
	postconditions[0] = NewPostconditionRefinement(path.NewPath(symbol.ID(65), "mutated"), valueRefinementWithPresence(presence.Absent()))
	postconditionsAgain := facts.PostconditionRefinements(point)
	assertPathEqual(t, postconditionsAgain[0].TargetPath(), path.NewPath(symbol.ID(63), "post").Field("value"))

	ret, ok := facts.Return(point)
	if !ok {
		t.Fatal("return missing")
	}
	retSources := ret.Sources()
	if len(retSources) != 2 || retSources[0].Kind != ValueSourceExpression {
		t.Fatalf("return sources = %#v", retSources)
	}
	retSources[0].Kind = ValueSourceNil
	retAgain, _ := facts.Return(point)
	if got := retAgain.Sources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("facts return exposed mutable sources, got %v", got[0].Kind)
	}

	callSite, ok := facts.CallSite(point)
	if !ok {
		t.Fatal("call site missing")
	}
	if callSite.Context() != CallSiteContextAssignmentSource || callSite.CalleeSymbol() != symbol.ID(35) || callSite.ExprIndex() != 1 {
		t.Fatalf("call site context/symbol/expr index = %v/%v/%v", callSite.Context(), callSite.CalleeSymbol(), callSite.ExprIndex())
	}
	callSiteCalleePath := callSite.CalleePath()
	callSiteCalleePath.Segments[0].Name = "mutated"
	callSiteAgain, _ := facts.CallSite(point)
	assertDirectField(t, callSiteAgain.CalleePath(), "site")
	callSiteReceiverPath, ok := callSite.ReceiverPath()
	if !ok || !callSiteReceiverPath.Equal(path.NewPath(symbol.ID(35), "callee")) {
		t.Fatalf("call site receiver path = %#v/%v", callSiteReceiverPath, ok)
	}
	callSiteReceiverPath.Root = "mutated"
	callSiteAgain, _ = facts.CallSite(point)
	if receiverPath, ok := callSiteAgain.ReceiverPath(); !ok || !receiverPath.Equal(path.NewPath(symbol.ID(35), "callee")) {
		t.Fatalf("facts call site exposed mutable receiver path: %#v/%v", receiverPath, ok)
	}
	callSiteMethodPath, ok := callSite.MethodPath()
	if !ok || !callSiteMethodPath.Equal(path.NewPath(symbol.ID(35), "callee").Field("site")) {
		t.Fatalf("call site method path = %#v/%v", callSiteMethodPath, ok)
	}
	callSiteMethodPath.Segments[0].Name = "mutated"
	callSiteAgain, _ = facts.CallSite(point)
	if methodPath, ok := callSiteAgain.MethodPath(); !ok || !methodPath.Equal(path.NewPath(symbol.ID(35), "callee").Field("site")) {
		t.Fatalf("facts call site exposed mutable method path: %#v/%v", methodPath, ok)
	}
	if callSite.MethodName() != "site" {
		t.Fatalf("call site method name = %q, want site", callSite.MethodName())
	}
	callSiteArgs := callSite.ArgumentSources()
	if len(callSiteArgs) != 2 || callSiteArgs[0].Kind != ValueSourceExpression || callSiteArgs[1].Kind != ValueSourceCall {
		t.Fatalf("call site argument sources = %#v", callSiteArgs)
	}
	callSiteArgs[0].Kind = ValueSourceNil
	callSiteAgain, _ = facts.CallSite(point)
	if got := callSiteAgain.ArgumentSources(); got[0].Kind != ValueSourceExpression {
		t.Fatalf("facts call site exposed mutable argument sources, got %v", got[0].Kind)
	}
	callSiteTypeArgs := callSite.TypeArgs()
	if len(callSiteTypeArgs) != 2 || callSiteTypeArgs[0] != TypeRef(7) || callSiteTypeArgs[1] != TypeRef(8) {
		t.Fatalf("call site type args = %#v", callSiteTypeArgs)
	}
	callSiteTypeArgs[0] = TypeRef(99)
	callSiteAgain, _ = facts.CallSite(point)
	if got := callSiteAgain.TypeArgs(); got[0] != TypeRef(7) {
		t.Fatalf("facts call site exposed mutable type args, got %#v", got)
	}
	callSiteTargets := callSite.ResultTargets()
	if len(callSiteTargets) != 1 || callSiteTargets[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("call site targets = %#v", callSiteTargets)
	}
	callSiteTargetPath := callSiteTargets[0].TargetPath()
	callSiteTargetPath.Root = "mutated"
	assertPathEqual(t, callSiteAgain.ResultTargets()[0].TargetPath(), path.NewPath(symbol.ID(33), "table"))
	callSiteTargets[0] = NewCallResultTarget(CallResultTargetReturn, 0, 0, 0, path.Path{})
	if got := callSiteAgain.ResultTargets(); got[0].Kind() != CallResultTargetLocalAssignment {
		t.Fatalf("facts call site exposed mutable targets, got %v", got[0].Kind())
	}
	callSiteView, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatal("call site view missing")
	}
	if callSiteView.Context() != CallSiteContextAssignmentSource || callSiteView.CalleeSymbol() != symbol.ID(35) || callSiteView.ExprIndex() != 1 {
		t.Fatalf("call site view context/symbol/expr index = %v/%v/%v", callSiteView.Context(), callSiteView.CalleeSymbol(), callSiteView.ExprIndex())
	}
	if receiverPath, ok := callSiteView.ReceiverPath(); !ok || !receiverPath.Equal(path.NewPath(symbol.ID(35), "callee")) {
		t.Fatalf("call site view receiver path = %#v/%v", receiverPath, ok)
	}
	if methodPath, ok := callSiteView.MethodPath(); !ok || !methodPath.Equal(path.NewPath(symbol.ID(35), "callee").Field("site")) {
		t.Fatalf("call site view method path = %#v/%v", methodPath, ok)
	}
	if callSiteView.MethodName() != "site" {
		t.Fatalf("call site view method name = %q, want site", callSiteView.MethodName())
	}
	if expr, ok := callSiteView.Expr(); !ok || expr != ExprRef(5) {
		t.Fatalf("call site view expr = %v/%v, want %v/true", expr, ok, ExprRef(5))
	}
	if !callSiteView.Final() || !callSiteView.Expanded() || callSiteView.Adjusted() || callSiteView.OpenTail() {
		t.Fatalf("call site view flags were not preserved")
	}
	if args := callSiteView.ArgumentSources(); len(args) != 2 || args[0].Kind != ValueSourceExpression || args[1].Kind != ValueSourceCall {
		t.Fatalf("call site view args = %#v", args)
	}
	if typeArgs := callSiteView.TypeArgs(); len(typeArgs) != 2 || typeArgs[0] != TypeRef(7) || typeArgs[1] != TypeRef(8) {
		t.Fatalf("call site view type args = %#v", typeArgs)
	}
	copiedSite := callSiteView.CallSite()
	copiedSite.calleePath.Segments[0].Name = "mutated-through-view-copy"
	callSiteAgain, _ = facts.CallSite(point)
	assertDirectField(t, callSiteAgain.CalleePath(), "site")
	visitedTargets := 0
	callSiteView.ForEachResultTarget(func(target CallResultTargetView) bool {
		visitedTargets++
		if target.Kind() != CallResultTargetLocalAssignment || target.TargetPathKey() == "" {
			t.Fatalf("call site view target = %v/%q, want local assignment with path key", target.Kind(), target.TargetPathKey())
		}
		targetPath := target.TargetPath()
		targetPath.Root = "mutated-through-view-target-copy"
		return true
	})
	if visitedTargets != 1 {
		t.Fatalf("call site view visited %d targets, want 1", visitedTargets)
	}
	callSiteAgain, _ = facts.CallSite(point)
	assertPathEqual(t, callSiteAgain.ResultTargets()[0].TargetPath(), path.NewPath(symbol.ID(33), "table"))

	literal, ok := facts.ObjectLiteral(ExprRef(1))
	if !ok {
		t.Fatal("object literal missing")
	}
	entries := literal.Entries()
	if len(entries) != 1 || entries[0].Source() != source {
		t.Fatalf("object literal entries = %#v", entries)
	}
	entrySuffix := entries[0].Suffix()
	entrySuffix.Segments[0].Name = "mutated"
	literalAgain, _ := facts.ObjectLiteral(ExprRef(1))
	if got := literalAgain.Entries()[0].Suffix(); got.Segments[0].Name != "field" {
		t.Fatalf("facts object literal exposed mutable suffix: %#v", got)
	}

	exprValue, ok := facts.ExpressionValue(ExprRef(3))
	if !ok {
		t.Fatal("expression value missing")
	}
	if got := product.Get(standard.Registry(), exprValue, runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("expression value runtime kind = %s, want string", got)
	}
	allExpressionValues := facts.ExpressionValues()
	allExpressionValues[ExprRef(3)] = product.Set(standard.Registry(), product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	allExpressionValuesAgain := facts.ExpressionValues()
	if got := product.Get(standard.Registry(), allExpressionValuesAgain[ExprRef(3)], runtimekind.Key); !runtimekind.Equal(got, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("facts expression values exposed mutable map, got runtime kind %s", got)
	}

	refinementFact, ok := facts.ExpressionRefinement(ExprRef(4))
	if !ok {
		t.Fatal("expression refinement missing")
	}
	wantRefinement := runtimeKindConstraint(runtimekind.Singleton(runtimekind.Table))
	if refinementFact.Source() != source || !product.Equal(standard.Registry(), refinementFact.Refinement(), wantRefinement) {
		t.Fatalf("expression refinement fact = %#v, want original refinement", refinementFact)
	}

	exprPath, ok := facts.ExpressionPath(ExprRef(6))
	if !ok {
		t.Fatal("expression path missing")
	}
	assertPathEqual(t, exprPath, path.NewPath(symbol.ID(54), "read").Field("leaf"))
	exprPath.Segments[0].Name = "mutated"
	exprPathAgain, _ := facts.ExpressionPath(ExprRef(6))
	assertDirectField(t, exprPathAgain, "leaf")
	allExpressionPaths := facts.ExpressionPaths()
	allExpressionPaths[ExprRef(6)] = path.NewPath(symbol.ID(56), "mutated").Field("other")
	allExpressionPathsAgain := facts.ExpressionPaths()
	assertDirectField(t, allExpressionPathsAgain[ExprRef(6)], "leaf")
}

func TestWithPathDescendantInvalidationsKeepsExistingPathOnCollision(t *testing.T) {
	point := cfg.Point(30)
	other := cfg.Point(31)
	firstPath := path.NewPath(symbol.ID(70), "first")
	secondPath := path.NewPath(symbol.ID(71), "second")
	otherPath := path.NewPath(symbol.ID(72), "other")

	facts := NewFacts(FactsInput{
		PathDescendantInvalidations: map[cfg.Point]PathDescendantInvalidation{
			point: NewPathDescendantInvalidation(firstPath),
		},
	})
	got := facts.WithPathDescendantInvalidations(map[cfg.Point]PathDescendantInvalidation{
		point: NewPathDescendantInvalidation(secondPath),
		other: NewPathDescendantInvalidation(otherPath),
	})

	invalidation, ok := got.PathDescendantInvalidation(point)
	if !ok {
		t.Fatalf("missing original invalidation")
	}
	assertPathEqual(t, invalidation.ContainerPath(), firstPath)
	otherInvalidation, ok := got.PathDescendantInvalidation(other)
	if !ok {
		t.Fatalf("missing merged invalidation")
	}
	assertPathEqual(t, otherInvalidation.ContainerPath(), otherPath)
	if _, ok := facts.PathDescendantInvalidation(other); ok {
		t.Fatalf("source facts were mutated")
	}
}

func assertDirectField(t *testing.T, p path.Path, want string) {
	t.Helper()
	got, ok := p.DirectFieldName()
	if !ok || got != want {
		t.Fatalf("path %q direct field = %q/%v, want %q/true", p.String(), got, ok, want)
	}
}

func assertPathEqual(t *testing.T, got path.Path, want path.Path) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("path = %q, want %q", got.String(), want.String())
	}
}

func valueRefinementWithPresence(value presence.Value) ValueRefinement {
	return NewValueConstraint(presenceConstraint(value))
}

func valueRefinementWithPresenceRuntime(p presence.Value, kind runtimekind.Value) ValueRefinement {
	reg := standard.Registry()
	return NewValueRefinement().
		WithConstraint(reg, presenceConstraint(p)).
		WithConstraint(reg, runtimeKindConstraint(kind))
}

func presenceConstraint(value presence.Value) product.Value {
	return product.NewWithPresence(standard.Registry(), product.ShapeTop, value)
}

func runtimeKindConstraint(value runtimekind.Value) product.Value {
	return product.Set(standard.Registry(), product.Top(), runtimekind.Key, value)
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	default:
		return product.PresenceOf(v).String()
	}
}

func assertValueRefinementConstraint(
	t *testing.T,
	label string,
	got ValueRefinement,
	wantPresence presence.Value,
	wantRuntimeKind runtimekind.Value,
) {
	t.Helper()
	constraint, ok := got.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	if gotPresence := product.PresenceOf(constraint); !presence.Equal(gotPresence, wantPresence) {
		t.Fatalf("%s presence = %s, want %s", label, gotPresence, wantPresence)
	}
	if gotRuntimeKind := product.Get(standard.Registry(), constraint, runtimekind.Key); !runtimekind.Equal(gotRuntimeKind, wantRuntimeKind) {
		t.Fatalf("%s runtime kind = %s, want %s", label, gotRuntimeKind, wantRuntimeKind)
	}
}
