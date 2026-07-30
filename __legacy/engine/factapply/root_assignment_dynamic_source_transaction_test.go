package factapply

import (
	"context"
	"errors"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func prepareDynamicRootSidecarFixture(t *testing.T) (state.ProductDomain, RootAssignmentDynamicSourcePlan, state.LaneFactor, state.LaneFactor, string, string) {
	t.Helper()
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(860)
	target, table, keySymbol := symbol.ID(860), symbol.ID(861), symbol.ID(862)
	targetPath := pathdom.NewPath(target, "target")
	tablePath := pathdom.NewPath(table, "table")
	keyPath := pathdom.NewPath(keySymbol, "key")
	keyRef, sourceRef := factflow.ExprRef(861), factflow.ExprRef(862)
	keySource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: keyRef, HasExpr: true}
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: sourceRef, HasExpr: true}
	dynamic, ok := factflow.NewDynamicIndexExpression(tablePath, keySource)
	if !ok {
		t.Fatal("dynamic expression")
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{sourceRef: dynamic},
		ExpressionPaths:         map[factflow.ExprRef]pathdom.Path{keyRef: keyPath},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	builder.Define(point, table, "table")
	builder.Define(point, keySymbol, "key")
	resolver := visibility.NewResolver(builder.Build())
	plan, present, err := PrepareRootAssignmentDynamicSourcePlan(domain, resolver, facts, point, targetPath, source)
	if err != nil || !present {
		t.Fatalf("prepare plan = %v/%v", present, err)
	}
	if plan.targetState == "" || plan.readKey == "" || len(plan.containers) == 0 {
		t.Fatalf("incomplete dynamic sidecar addresses: target=%q read=%q containers=%d", plan.targetState, plan.readKey, len(plan.containers))
	}
	if got, present := plan.KeyValueInput(); !present || got != keySource {
		t.Fatalf("key input = %#v/%t, want %#v", got, present, keySource)
	}
	container, ok := visibility.AddressAt(resolver, point, tablePath).VisibleKeyspaceKey()
	if !ok {
		t.Fatal("container key")
	}
	targetState, _ := visibility.AddressAt(resolver, point, targetPath).VisibleStateKey()
	tableState, _ := visibility.AddressAt(resolver, point, tablePath).VisibleStateKey()
	presentValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	current := state.Reachable(state.State{}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: container, Site: "site"}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{Value: presentValue, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})).
		AddDynamicIndexValueKeyMembership(container, "site", tableState)
	dynamicLane, _ := domain.ProductLane(state.LaneDynamicIndex)
	membershipLane, _ := domain.ProductLane(state.LaneKeyMemberships)
	factors, err := domain.DecomposeLanes(current, []state.ProductLane{dynamicLane, membershipLane})
	if err != nil {
		t.Fatal(err)
	}
	return domain, plan, factors[0], factors[1], string(targetState), string(tableState)
}

func TestRootAssignmentDynamicSourceKeyValueInputRejectsZeroPlan(t *testing.T) {
	if source, present := (RootAssignmentDynamicSourcePlan{}).KeyValueInput(); present || source != (factflow.ValueSource{}) {
		t.Fatalf("zero-plan key input = %#v/%t", source, present)
	}
}

func TestRootAssignmentDynamicSourceUnproductiveRootStillPublishesOriginAndMembership(t *testing.T) {
	domain, plan, dynamicFactor, membershipFactor, targetState, tableState := prepareDynamicRootSidecarFixture(t)
	transaction, err := plan.Resolve(context.Background(), RootAssignmentDynamicSourceInputs{
		DynamicIndexFactor: dynamicFactor, KeyMembershipFactor: membershipFactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	gotFactor, err := transaction.ApplyDynamicSourceFactor(membershipFactor)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeSparse([]state.LaneFactor{gotFactor})
	if err != nil {
		t.Fatal(err)
	}
	if origins := got.DynamicIndexReadOriginsForValue(pathStateKey(targetState)); len(origins) == 0 {
		t.Fatalf("Applied=false lost dynamic read origin: target=%q planTarget=%q origins=%#v", targetState, plan.targetState, origins)
	}
	if !got.HasPathKeyMembership(pathStateKey(targetState), pathStateKey(tableState)) {
		t.Fatal("Applied=false lost propagated key membership")
	}
}

func TestRootAssignmentDynamicSourcePublishesStaticKeyEquality(t *testing.T) {
	domain, plan, dynamicFactor, membershipFactor, _, _ := prepareDynamicRootSidecarFixture(t)
	transaction, err := plan.Resolve(context.Background(), RootAssignmentDynamicSourceInputs{
		KeyValue: typevalue.LiteralString(domain.Registry(), "name"), HasKeyValue: true,
		DynamicIndexFactor: dynamicFactor, KeyMembershipFactor: membershipFactor,
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, ok := transaction.PublishedEqualityProof()
	if !ok || proof.Kind != pathevidence.BranchProofPathEqual || proof.Path.Kind == 0 || proof.Other.Kind == 0 || proof.Path == proof.Other {
		t.Fatalf("static-key equality = %#v/%v", proof, ok)
	}
}

func TestRootAssignmentDynamicSourceModuloPresenceRemovesNil(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(870)
	target, table := symbol.ID(870), symbol.ID(871)
	targetPath, tablePath := pathdom.NewPath(target, "target"), pathdom.NewPath(table, "table")
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	base, _ := factflow.NewExpressionValueSource(871, -1, -1, 0, shape)
	tableOperand, _ := factflow.NewExpressionValueSource(872, -1, -1, 0, shape)
	length, _ := factflow.NewExpressionValueSource(873, -1, -1, 0, shape)
	modulo, _ := factflow.NewExpressionValueSource(874, -1, -1, 0, shape)
	one, _ := factflow.NewIntegerLiteralValueSource(1, -1, -1, 0, shape)
	keySource, _ := factflow.NewExpressionValueSource(875, -1, -1, 0, shape)
	source, _ := factflow.NewExpressionValueSource(876, -1, -1, 0, shape)
	lengthOp, _ := factflow.NewUnaryExpressionOperation("#", tableOperand)
	moduloOp, _ := factflow.NewBinaryExpressionOperation("%", base, length)
	keyOp, _ := factflow.NewBinaryExpressionOperation("+", modulo, one)
	dynamic, _ := factflow.NewDynamicIndexExpression(tablePath, keySource)
	facts := factflow.NewFacts(factflow.FactsInput{
		DynamicIndexExpressions: map[factflow.ExprRef]factflow.DynamicIndexExpression{source.ExprRef: dynamic},
		ExpressionOperations: map[factflow.ExprRef]factflow.ExpressionOperation{
			length.ExprRef: lengthOp, modulo.ExprRef: moduloOp, keySource.ExprRef: keyOp,
		},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{tableOperand.ExprRef: tablePath},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	builder.Define(point, table, "table")
	resolver := visibility.NewResolver(builder.Build())
	plan, present, err := PrepareRootAssignmentDynamicSourcePlan(domain, resolver, facts, point, targetPath, source)
	if err != nil || !present {
		t.Fatalf("plan = %v/%v", present, err)
	}
	if gotPath, gotBase, ok := plan.ModuloLengthPresenceInput(); !ok || !gotPath.Equal(tablePath) || gotBase != base {
		t.Fatalf("modulo input = %#v/%#v/%v", gotPath, gotBase, ok)
	}
	query, queryPresent, err := plan.TableNonEmptyQuery()
	if err != nil || !queryPresent {
		t.Fatalf("table query = %t/%v", queryPresent, err)
	}
	lenSlot, _ := query.LenFloorSlot()
	refinementSlot, _ := query.RefinementSlot()
	staticSlot, _ := query.StaticMemberSlot()
	rootSlot, rootPresent := query.RootValueSlot()
	if !rootPresent || rootSlot != statekey.SymbolValue(table) {
		t.Fatalf("root Values slot = %v/%t", rootSlot, rootPresent)
	}
	tableState, _ := visibility.AddressAt(resolver, point, tablePath).VisibleStateKey()
	queryState := state.Reachable(state.State{}).WriteLenFloor(resolver.KeySpace(), tableState, 1)
	lenFamily, _ := domain.LenFloorCoordinateFamily()
	lenLane := lenFamily.Lane()
	lenFactors, _ := domain.DecomposeLanes(queryState, []state.ProductLane{lenLane})
	lenSkeleton, lenScalars, err := domain.DecomposeCoordinateFamily(lenFactors[0], lenFamily, resolver.KeySpace())
	if err != nil || len(lenScalars) != 1 {
		t.Fatalf("length query scalar = %d/%v", len(lenScalars), err)
	}
	_ = lenSkeleton
	pathFamily, _ := domain.PathEvidenceCoordinateFamily()
	pathFactors, _ := domain.DecomposeLanes(queryState, []state.ProductLane{pathFamily.Lane()})
	pathSkeleton, _, err := domain.DecomposeCoordinateFamily(pathFactors[0], pathFamily, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	refinement, err := domain.CoordinateDefault(pathSkeleton, refinementSlot)
	if err != nil {
		t.Fatal(err)
	}
	staticMember, err := domain.CoordinateDefault(pathSkeleton, staticSlot)
	if err != nil {
		t.Fatal(err)
	}
	if equal, err := domain.CoordinateSlotEqual(lenScalars[0].Slot(), lenSlot); err != nil || !equal {
		t.Fatalf("length slot mismatch = %t/%v", equal, err)
	}
	nonempty, err := query.DefinitelyNonEmpty(typevalue.NewCache(), RootAssignmentTableNonEmptyInputs{LenFloor: lenScalars[0], Refinement: refinement, StaticMember: staticMember, RootValue: product.Bottom(reg), HasRootValue: true})
	if err != nil || !nonempty {
		t.Fatalf("length-proved nonempty = %t/%v", nonempty, err)
	}
	if _, err := query.DefinitelyNonEmpty(typevalue.NewCache(), RootAssignmentTableNonEmptyInputs{LenFloor: lenScalars[0], Refinement: refinement, StaticMember: staticMember}); err == nil {
		t.Fatal("length proof bypassed required root Values operand")
	}
	rootType := typevalue.NewCache()
	rootValue := rootType.FromTypeWithWitness(reg, typ.NewTuple(typ.String))
	emptyLenFactors, _ := domain.DecomposeLanes(state.Reachable(state.State{}), []state.ProductLane{lenLane})
	emptyLenSkeleton, _, err := domain.DecomposeCoordinateFamily(emptyLenFactors[0], lenFamily, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	lenDefault, err := domain.CoordinateDefault(emptyLenSkeleton, lenSlot)
	if err != nil {
		t.Fatal(err)
	}
	nonempty, err = query.DefinitelyNonEmpty(rootType, RootAssignmentTableNonEmptyInputs{LenFloor: lenDefault, Refinement: refinement, StaticMember: staticMember, RootValue: rootValue, HasRootValue: true})
	if err != nil || !nonempty {
		t.Fatalf("root-value-proved nonempty = %t/%v", nonempty, err)
	}
	visibleTable, _ := visibility.AddressAt(resolver, point, tablePath).VisibleKeyspaceKey()
	refinedState := state.Reachable(state.State{}).WriteLocalPathKey(reg, visibleTable, rootValue)
	refinedFactors, _ := domain.DecomposeLanes(refinedState, []state.ProductLane{pathFamily.Lane()})
	_, refinedScalars, err := domain.DecomposeCoordinateFamily(refinedFactors[0], pathFamily, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	var refinedScalar state.CoordinateScalarFactor
	for _, scalar := range refinedScalars {
		equal, _ := domain.CoordinateSlotEqual(scalar.Slot(), refinementSlot)
		if equal {
			refinedScalar = scalar
		}
	}
	nonempty, err = query.DefinitelyNonEmpty(rootType, RootAssignmentTableNonEmptyInputs{LenFloor: lenDefault, Refinement: refinedScalar, StaticMember: staticMember, RootValue: product.Bottom(reg), HasRootValue: true})
	if err != nil || !nonempty {
		t.Fatalf("refinement-proved nonempty = %t/%v", nonempty, err)
	}
	if _, err := query.DefinitelyNonEmpty(rootType, RootAssignmentTableNonEmptyInputs{LenFloor: lenDefault, Refinement: staticMember, StaticMember: refinement, RootValue: product.Bottom(reg), HasRootValue: true}); err == nil {
		t.Fatal("table query admitted swapped path coordinate operands")
	}
	dynamicLane, _ := domain.ProductLane(state.LaneDynamicIndex)
	membershipLane, _ := domain.ProductLane(state.LaneKeyMemberships)
	factors, _ := domain.DecomposeLanes(state.Reachable(state.State{}), []state.ProductLane{dynamicLane, membershipLane})
	transaction, err := plan.Resolve(context.Background(), RootAssignmentDynamicSourceInputs{
		ModuloBaseValue: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Integer), typ.Integer), HasModuloBaseValue: true,
		TableDefinitelyNonEmpty: true, DynamicIndexFactor: factors[0], KeyMembershipFactor: factors[1],
	})
	if err != nil {
		t.Fatal(err)
	}
	optional := typevalue.FromType(reg, typeexpr.Optional(typ.String))
	got, productive, err := transaction.ComposeSourceValue(optional, true, RootAssignmentSourceComposition{})
	if err != nil || !productive || !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("composed source = productive:%v presence:%v err:%v", productive, product.PresenceOf(got), err)
	}
}

func TestRootAssignmentDynamicSourceResolutionIsCancelAndErrorAtomic(t *testing.T) {
	_, plan, dynamicFactor, membershipFactor, _, _ := prepareDynamicRootSidecarFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transaction, err := plan.Resolve(ctx, RootAssignmentDynamicSourceInputs{
		DynamicIndexFactor: dynamicFactor, KeyMembershipFactor: membershipFactor,
	})
	if !errors.Is(err, context.Canceled) || transaction.Valid() {
		t.Fatalf("canceled resolution = valid:%v err:%v", transaction.Valid(), err)
	}
	transaction, err = plan.Resolve(context.Background(), RootAssignmentDynamicSourceInputs{
		DynamicIndexFactor: membershipFactor, KeyMembershipFactor: dynamicFactor,
	})
	if err == nil || transaction.Valid() {
		t.Fatalf("malformed resolution = valid:%v err:%v", transaction.Valid(), err)
	}
}

func pathStateKey(value string) pathaddr.StateKey { return pathaddr.StateKey(value) }
