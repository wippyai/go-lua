package factapply

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func prepareRootAssignmentFactorFixture(
	t *testing.T,
	point cfg.Point,
	target symbol.ID,
	assignment factflow.RootAssignment,
	value product.Value,
) (*RootAssignmentAuthority, ResolvedRootAssignmentPlan, ResolvedRootAssignmentTransaction) {
	t.Helper()
	reg := standard.Registry()
	facts := factflow.NewFacts(factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{point: assignment}})
	builder := visibility.NewBuilder()
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	authority := NewRootAssignmentAuthority(
		NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()),
		facts,
		nil,
		state.RegisteredProductDomain(reg),
	)
	transaction, ok := PlanRootAssignmentTransaction(facts, point)
	if !ok {
		t.Fatal("root-assignment transaction missing")
	}
	plan, err := authority.PrepareResolvedRootAssignmentPlan(transaction)
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := transaction.Bind(reg, []product.Value{value})
	if !ok {
		t.Fatal("root-assignment transaction binding failed")
	}
	return authority, plan, resolved
}

func TestResolvedRootAssignmentPlanComposesExactBottomAndDeclaredSources(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(501)
	target := symbol.ID(501)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(point), HasExpr: true}
	targetPath := pathdom.NewPath(target, "target")
	exact := typevalue.LiteralString(reg, "exact-root-value")
	ordinary := factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, source)

	authority, plan, _ := prepareRootAssignmentFactorFixture(t, point, target, ordinary, exact)
	got, productive, err := plan.ComposeFactorPrimarySourceValue(reg, exact, false)
	if err != nil || !productive || !product.Equal(reg, got, exact) {
		t.Fatalf("exact factor composition = productive:%t value:%v err:%v", productive, got, err)
	}
	write, err := plan.PrepareFactorValueWrite(got)
	if err != nil {
		t.Fatal(err)
	}
	written, err := authority.domain.ApplyRootAssignmentValueScalar(write, false)
	if err != nil || !product.Equal(reg, written, exact) {
		t.Fatalf("exact factor value write = %v, err %v", written, err)
	}

	_, bottomPlan, _ := prepareRootAssignmentFactorFixture(t, point+1, target+1,
		factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target+1, pathdom.NewPath(target+1, "bottom"), source),
		product.Bottom(reg),
	)
	if _, productive, err := bottomPlan.ComposeFactorPrimarySourceValue(reg, product.Bottom(reg), false); err != nil || productive {
		t.Fatalf("Bottom factor composition = productive:%t err:%v, want false/nil", productive, err)
	}

	declared := typevalue.LiteralString(reg, "declared-contract")
	declaredAssignment := factflow.NewRootAssignmentWithDeclaredContractValue(
		factflow.RootAssignmentLocalDeclaration, target+2, pathdom.NewPath(target+2, "declared"), source, declared,
	)
	_, declaredPlan, _ := prepareRootAssignmentFactorFixture(t, point+2, target+2, declaredAssignment, product.Bottom(reg))
	got, productive, err = declaredPlan.ComposeFactorPrimarySourceValue(reg, product.Bottom(reg), false)
	if err != nil || !productive || !product.Equal(reg, got, declared) {
		t.Fatalf("declared factor composition = productive:%t value:%v err:%v", productive, got, err)
	}
	foreign, registryErr := standard.RegistryWithAxes()
	if registryErr != nil {
		t.Fatal(registryErr)
	}
	if _, _, err := declaredPlan.ComposeFactorPrimarySourceValue(reg, typevalue.FromType(foreign, typ.String), false); err == nil {
		t.Fatal("factor source composition admitted a foreign primary source")
	}
}

func TestCallReceiverCompletionUsesOnlyRegisteredFactorLaws(t *testing.T) {
	reg := standard.Registry()
	point, callPoint := cfg.Point(505), cfg.Point(504)
	target := symbol.ID(505)
	targetPath := pathdom.NewPath(target, "tuple-result")
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, 0, 0, callPoint, shape)
	if !ok {
		t.Fatal("call source rejected")
	}
	assignment := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source)
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.NewTuple(typ.String, typ.Number))
	authority, _, resolved := prepareRootAssignmentFactorFixture(t, point, target, assignment, value)
	domain := authority.domain
	targetStateKey, ok := visibility.AddressAt(authority.paths.resolver, point, targetPath).VisibleStateKey()
	if !ok {
		t.Fatal("target state key missing")
	}
	otherStateKey := pathaddr.StateKey("sym506@1.other")
	current := state.State{}.
		WriteValue(reg, key.SymbolValue(target), value).
		WriteNumFloor(authority.paths.resolver.KeySpace(), targetStateKey, 7).
		WriteNumCeil(authority.paths.resolver.KeySpace(), targetStateKey, 9).
		WriteDiffConstraint(state.RelValueOperand(targetStateKey), state.RelValueOperand(otherStateKey), 3)
	transaction, err := authority.PrepareCallReceiverFactorTransaction(reg, resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := applyRootAssignmentCompletionFactorsForTest(t, domain, authority.paths.resolver.KeySpace(), transaction, current)
	if floor, present := got.ReadLenFloor(authority.paths.resolver.KeySpace(), targetStateKey); !present || floor != 2 {
		t.Fatalf("tuple length floor = %d/%t, want 2/true", floor, present)
	}
	if floor, present := got.ReadNumFloor(authority.paths.resolver.KeySpace(), targetStateKey); !present || floor != 7 {
		t.Fatalf("transported numeric floor = %d/%t, want 7/true", floor, present)
	}
	if ceil, present := got.ReadNumCeil(authority.paths.resolver.KeySpace(), targetStateKey); !present || ceil != 9 {
		t.Fatalf("transported numeric ceiling = %d/%t, want 9/true", ceil, present)
	}
	constraints := got.RelConstraints().Constraints
	if len(constraints) != 1 || constraints[0].K != 3 {
		t.Fatalf("transported difference constraint changed: %+v", constraints)
	}
}

func TestScalarCallReceiverFactorCompletionIsIdentity(t *testing.T) {
	reg := standard.Registry()
	point, callPoint := cfg.Point(507), cfg.Point(506)
	target := symbol.ID(507)
	targetPath := pathdom.NewPath(target, "scalar-result")
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, _ := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, 0, 0, callPoint, shape)
	assignment := factflow.NewRootAssignment(factflow.RootAssignmentLocalDeclaration, target, targetPath, source)
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	authority, _, resolved := prepareRootAssignmentFactorFixture(t, point, target, assignment, value)
	current := state.Reachable(state.State{}).WriteValue(reg, key.SymbolValue(target), value)
	transaction, err := authority.PrepareCallReceiverFactorTransaction(reg, resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := applyRootAssignmentCompletionFactorsForTest(t, authority.domain, authority.paths.resolver.KeySpace(), transaction, current)
	if !authority.domain.Lattice().Equal(got, authority.domain.Normalize(current)) {
		t.Fatal("scalar call-receiver factor completion changed the product")
	}
}

func applyRootAssignmentCompletionFactorsForTest(
	t *testing.T,
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	transaction state.RootAssignmentFactorTransaction,
	current state.State,
) state.State {
	t.Helper()
	factors, err := domain.Decompose(current)
	if err != nil {
		t.Fatal(err)
	}
	for index := range factors {
		factors[index], err = domain.ApplyRootAssignmentCompletionFactor(transaction, factors[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, family := range domain.RootAssignmentCompletionCoordinateFamilies() {
		index := int(family.Lane().Ordinal())
		families, err := domain.CoordinateFamilies(family.Lane())
		if err != nil {
			t.Fatal(err)
		}
		skeletons := make([]state.CoordinateFamilySkeleton, len(families))
		scalars := make([][]state.CoordinateScalarFactor, len(families))
		for familyIndex, candidate := range families {
			skeletons[familyIndex], scalars[familyIndex], err = domain.DecomposeCoordinateFamily(factors[index], candidate, keys)
			if err != nil {
				t.Fatal(err)
			}
		}
		familyIndex := int(family.Ordinal())
		slot, present, err := domain.RootAssignmentCompletionCoordinateSlot(transaction, family, keys)
		if err != nil {
			t.Fatal(err)
		}
		if !present {
			continue
		}
		current, err := domain.CoordinateDefault(skeletons[familyIndex], slot)
		if err != nil {
			t.Fatal(err)
		}
		for _, scalar := range scalars[familyIndex] {
			equal, equalErr := domain.CoordinateSlotEqual(scalar.Slot(), slot)
			if equalErr != nil {
				t.Fatal(equalErr)
			}
			if equal {
				current = scalar
				break
			}
		}
		skeletons[familyIndex], current, err = domain.ApplyRootAssignmentCompletionCoordinate(transaction, skeletons[familyIndex], current)
		if err != nil {
			t.Fatal(err)
		}
		next := make([]state.CoordinateScalarFactor, 0, len(scalars[familyIndex])+1)
		replaced := false
		for _, scalar := range scalars[familyIndex] {
			equal, _ := domain.CoordinateSlotEqual(scalar.Slot(), slot)
			if equal {
				next = append(next, current)
				replaced = true
			} else {
				next = append(next, scalar)
			}
		}
		if !replaced {
			next = append(next, current)
			sort.Slice(next, func(i, j int) bool { less, _ := domain.CoordinateSlotLess(next[i].Slot(), next[j].Slot()); return less })
		}
		scalars[familyIndex] = next
		factors[index], err = domain.ComposeCoordinateFamilies(family.Lane(), keys, skeletons, scalars)
		if err != nil {
			t.Fatal(err)
		}
	}
	out, err := domain.Compose(factors)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
