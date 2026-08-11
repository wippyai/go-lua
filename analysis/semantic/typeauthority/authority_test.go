package typeauthority_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestAuthorityMaterializesDeepAcyclicProjectionWithoutSemanticDepthCap(t *testing.T) {
	const depth = 4097
	var source strings.Builder
	source.WriteString("type Deep = ")
	for index := 0; index < depth; index++ {
		source.WriteString("readonly {")
	}
	source.WriteString("string")
	for index := 0; index < depth; index++ {
		source.WriteByte('}')
	}
	p := lower(t, source.String())
	authority := seal(t, p)
	value, ok := authority.Materialize(selectorFor(t, authority, p, alias(t, p, 0)))
	if !ok || value == nil {
		t.Fatal("deep acyclic sealed type did not materialize")
	}
	for index := 0; index < depth; index++ {
		alias, isAlias := value.(*typ.Alias)
		if isAlias {
			value = alias.Target
		}
		view, isView := value.(*typ.ReadonlyMap)
		if !isView || view.Key != typ.Integer {
			t.Fatalf("layer %d = %T, want readonly array projection", index, value)
		}
		value = view.Value
	}
	if value != typ.String {
		t.Fatalf("deep leaf = %v, want string", value)
	}
}

func TestAuthorityUsesProgramRootsAsDensePortableSelectors(t *testing.T) {
	p := lower(t, `
type Choice = nil | { tag: "left" } | { tag: "right" }
type Shape = readonly { name: string, count?: integer }
type Fn = fun(value: Shape, ... number): string
`)
	authority := seal(t, p)
	if authority.Count() != p.Static().StaticTypes().Count() {
		t.Fatalf("selector count=%d, Program static terms=%d", authority.Count(), p.Static().StaticTypes().Count())
	}
	for index := 0; index < authority.Count(); index++ {
		selector, ok := authority.At(index)
		if !ok || selector == 0 {
			t.Fatalf("selector %d unavailable", index)
		}
		ref, ok := authority.Ref(selector)
		if !ok || ref.Owner() != p.ContentID() {
			t.Fatalf("selector %d ref=%v/%v", index, ref, ok)
		}
		if _, valid := p.Static().StaticTypes().Ref(ref.Root()); !valid {
			t.Fatalf("selector %d ref root=%v is not a Static term", index, ref.Root())
		}
		if roundTrip, ok := authority.Lookup(ref); !ok || roundTrip != selector {
			t.Fatalf("selector %d lookup=%d/%v", selector, roundTrip, ok)
		}
	}

	choice := alias(t, p, 0)
	_, choiceRoot, _, _, _ := p.Static().Declarations().Aliases().Get(choice)
	choiceSelector := selectorFor(t, authority, p, choiceRoot)
	family, ok := authority.Family(choiceSelector)
	if !ok || authority.FamilyArity(family) != 2 {
		t.Fatalf("choice family=%v/%v arity=%d", family, ok, authority.FamilyArity(family))
	}
	for index := 0; index < authority.FamilyArity(family); index++ {
		arm, ok := authority.FamilyArm(family, index)
		if !ok || arm == 0 {
			t.Fatalf("family arm %d=%d/%v", index, arm, ok)
		}
	}
}

func TestAuthorityMaterializesClosedProgramTypesAndGenericApplications(t *testing.T) {
	p := lower(t, `
type Choice = "left" | "right"
type Shape = readonly { name: string, count?: integer }
type Fn = fun(value: Shape, ... number): string
type Open<T: string> = { value: T }
type Applied = Open<string>
`)
	authority := seal(t, p)

	shape := alias(t, p, 1)
	shapeType, ok := authority.Materialize(selectorFor(t, authority, p, shape))
	if !ok {
		t.Fatal("closed readonly record did not materialize")
	}
	record, ok := shapeType.(*typ.Alias)
	if !ok {
		t.Fatalf("Shape=%T, want alias retaining diagnostic name", shapeType)
	}
	shapeRecord, ok := record.Target.(*typ.Record)
	if !ok {
		t.Fatalf("Shape target=%T, want record", record.Target)
	}
	count := shapeRecord.GetField("count")
	name := shapeRecord.GetField("name")
	if len(shapeRecord.Fields) != 2 || count == nil || name == nil || !count.Optional || !count.Readonly || !name.Readonly {
		t.Fatalf("Shape target=%#v", record.Target)
	}

	fn := alias(t, p, 2)
	fnType, ok := authority.Materialize(selectorFor(t, authority, p, fn))
	if !ok {
		t.Fatal("closed function type did not materialize")
	}
	if _, ok := fnType.(*typ.Alias); !ok {
		t.Fatalf("Fn=%T, want alias", fnType)
	}

	open := alias(t, p, 3)
	openType, ok := authority.Materialize(selectorFor(t, authority, p, open))
	if !ok {
		t.Fatal("generic declaration did not materialize")
	}
	generic, ok := openType.(*typ.Generic)
	if !ok || len(generic.TypeParams) != 1 {
		t.Fatalf("Open=%T (%v params), want generic with one formal", openType, len(generic.TypeParams))
	}
	if !typ.ContainsTypeParam(openType) {
		t.Fatal("open generic declaration was treated as closed")
	}
	if generic.TypeParams[0].Constraint != typ.String {
		t.Fatalf("Open formal constraint=%v, want string", generic.TypeParams[0].Constraint)
	}

	applied := alias(t, p, 4)
	appliedType, ok := authority.Materialize(selectorFor(t, authority, p, applied))
	if !ok {
		t.Fatal("closed generic application did not materialize")
	}
	appliedAlias, ok := appliedType.(*typ.Alias)
	if !ok {
		t.Fatalf("Applied=%T, want diagnostic alias", appliedType)
	}
	instance, ok := appliedAlias.Target.(*typ.Instantiated)
	if !ok || !typ.TypeEquals(instance.Generic, generic) || len(instance.TypeArgs) != 1 || instance.TypeArgs[0] != typ.String {
		t.Fatalf("Applied target=%#v, want Open<string>", appliedAlias.Target)
	}
	if typ.ContainsTypeParam(appliedType) {
		t.Fatal("fully substituted generic application remained open")
	}
}

func TestAuthorityPublicProjectionIsOwnershipIsolatedAndResolveFindUseRefs(t *testing.T) {
	p := lower(t, `type Shape = { name: string }`)
	authority := seal(t, p)
	declaration := alias(t, p, 0)
	selector := selectorFor(t, authority, p, declaration)
	ref, ok := authority.Ref(selector)
	if !ok {
		t.Fatal("missing authored ref")
	}
	if found, ok := authority.Lookup(ref); !ok || found != selector {
		t.Fatalf("Lookup=%d/%v, want %d", found, ok, selector)
	}
	first, ok := authority.Resolve(ref)
	if !ok {
		t.Fatal("Resolve failed")
	}
	alias, ok := first.(*typ.Alias)
	if !ok {
		t.Fatalf("first=%T, want alias", first)
	}
	alias.Target.(*typ.Record).Fields[0].Type = typ.Number
	second, ok := authority.Materialize(selector)
	if !ok {
		t.Fatal("Materialize failed")
	}
	field, ok := access.Field(second, "name")
	if !ok || field != typ.String {
		t.Fatalf("mutated public projection leaked: %v/%v", field, ok)
	}
}

func TestAuthorityRecursiveAliasIsMuSafeAndNeverRequiresAST(t *testing.T) {
	p := lower(t, `type Node = Node?`)
	authority := seal(t, p)
	node := alias(t, p, 0)
	value, ok := authority.Materialize(selectorFor(t, authority, p, node))
	if !ok {
		t.Fatal("recursive alias did not materialize")
	}
	if _, ok := value.(*typ.Recursive); !ok {
		t.Fatalf("recursive Node=%T, want *typ.Recursive", value)
	}
}

func TestAuthorityMaterializesGenericSignaturesAndStructuralInterfaces(t *testing.T) {
	p := lower(t, `
interface Base
  function map<T: string>(value: T): T
end
interface Shape: Base
  id: number
end
	`)
	authority := seal(t, p)

	shape, ok := p.Static().Declarations().Interfaces().At(1)
	if !ok {
		t.Fatal("missing Shape interface")
	}
	shapeType, ok := authority.Materialize(selectorFor(t, authority, p, shape))
	if !ok {
		t.Fatal("structural interface did not materialize")
	}
	if id, ok := access.Field(shapeType, "id"); !ok || id != typ.Number {
		t.Fatalf("Shape.id=%v/%v, want number", id, ok)
	}
	mapType, ok := access.Field(shapeType, "map")
	mapFunction, isFunction := mapType.(*typ.Function)
	if !ok || !isFunction || len(mapFunction.TypeParams) != 1 {
		t.Fatalf("Shape.map=%T/%v, want generic function", mapType, ok)
	}
	implementation := typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
		{Name: "id", Type: typ.Number},
		{Name: "map", Type: mapFunction},
	}})
	if !subtype.IsSubtype(implementation, shapeType) {
		t.Fatalf("record implementation does not satisfy materialized Shape")
	}
}

func TestAuthorityDoesNotEraseDuplicateInterfaceMembers(t *testing.T) {
	p := lower(t, `
interface Ambiguous
  id: string
  id: number
end
`)
	iface, ok := p.Static().Declarations().Interfaces().At(0)
	count, countOK := p.Static().Declarations().Interfaces().MemberCount(iface)
	if !ok || !countOK || count != 2 {
		t.Fatalf("Program did not retain duplicate interface rows: %v/%v count=%d/%v", iface, ok, count, countOK)
	}
	authority := seal(t, p)
	if value, ok := authority.Materialize(selectorFor(t, authority, p, iface)); ok || value != nil {
		t.Fatalf("duplicate interface materialized as %T; must remain Rule-owned", value)
	}
}

func TestAuthorityFamilyRequiresDirectClosedRecordUnion(t *testing.T) {
	p := lower(t, `
type Scalar = "left" | "right"
type Tagged = { tag: "only" }
type Mixed = { tag: "record" } | "not-a-record"
type Undiscriminated = { value: string } | { value: string }
`)
	authority := seal(t, p)
	for _, index := range []int{0, 1, 2, 3} {
		declaration := alias(t, p, index)
		_, target, _, _, ok := p.Static().Declarations().Aliases().Get(declaration)
		if !ok {
			t.Fatalf("missing alias target %d", index)
		}
		if family, ok := authority.Family(selectorFor(t, authority, p, target)); ok || family.Valid() {
			t.Fatalf("alias %d fabricated variant family %#v", index, family)
		}
	}
}

func TestAuthorityProjectsReadonlyArrayToExistingReadonlyIntegerMap(t *testing.T) {
	p := lower(t, `type View = readonly {string}`)
	authority := seal(t, p)
	view := alias(t, p, 0)
	value, ok := authority.Materialize(selectorFor(t, authority, p, view))
	if !ok {
		t.Fatal("readonly array did not materialize")
	}
	alias, ok := value.(*typ.Alias)
	if !ok {
		t.Fatalf("View=%T, want diagnostic alias", value)
	}
	mapView, ok := alias.Target.(*typ.ReadonlyMap)
	if !ok || mapView.Key != typ.Integer || mapView.Value != typ.String {
		t.Fatalf("View target=%#v, want readonly integer-keyed string map", alias.Target)
	}
}

func TestAuthoritySharesForwardFormalConstraintIdentity(t *testing.T) {
	p := lower(t, `type Pair<T: U, U: string> = { first: T, second: U }`)
	authority := seal(t, p)
	pair := alias(t, p, 0)
	value, ok := authority.Materialize(selectorFor(t, authority, p, pair))
	if !ok {
		t.Fatal("forward-constrained generic did not materialize")
	}
	generic, ok := value.(*typ.Generic)
	if !ok || len(generic.TypeParams) != 2 {
		t.Fatalf("Pair=%T, want two-formal generic", value)
	}
	if generic.TypeParams[0].Constraint != generic.TypeParams[1] || generic.TypeParams[1].Constraint != typ.String {
		t.Fatalf("Pair formal constraints=%v/%v, want T:U and U:string", generic.TypeParams[0].Constraint, generic.TypeParams[1].Constraint)
	}
}

func TestAuthoritySharesBackwardFormalConstraintIdentity(t *testing.T) {
	p := lower(t, `type Pair<T, U: T> = { first: T, second: U }`)
	authority := seal(t, p)
	pair := alias(t, p, 0)
	value, ok := authority.Materialize(selectorFor(t, authority, p, pair))
	if !ok {
		t.Fatal("backward-constrained generic did not materialize")
	}
	generic, ok := value.(*typ.Generic)
	if !ok || len(generic.TypeParams) != 2 {
		t.Fatalf("Pair=%T, want two-formal generic", value)
	}
	if generic.TypeParams[0].Constraint != nil || generic.TypeParams[1].Constraint != generic.TypeParams[0] {
		t.Fatalf("Pair formal constraints=%v/%v, want T:any and U:T", generic.TypeParams[0].Constraint, generic.TypeParams[1].Constraint)
	}
}

func TestAuthorityClosesOnlyArityAndDependentBoundValidApplications(t *testing.T) {
	p := lower(t, `
type Pair<T, U: T> = { first: T, second: U }
type Good = Pair<string, string>
type BadBound = Pair<string, number>
type BadArity = Pair<string>
`)
	authority := seal(t, p)

	good, ok := authority.Materialize(selectorFor(t, authority, p, alias(t, p, 1)))
	if !ok || good == nil || typ.ContainsTypeParam(good) {
		t.Fatalf("Good=%v/%v, want a closed application", good, ok)
	}
	if _, ok := good.(*typ.Alias); !ok {
		t.Fatalf("Good=%T, want diagnostic alias", good)
	}
	for index, name := range []string{"BadBound", "BadArity"} {
		value, materialized := authority.Materialize(selectorFor(t, authority, p, alias(t, p, index+2)))
		if materialized || value != nil {
			t.Fatalf("%s materialized as %T; invalid application must remain unavailable", name, value)
		}
	}
}

func TestAuthorityLeavesCyclicFormalBoundsSymbolic(t *testing.T) {
	p := lower(t, `type Pair<T: U, U: T> = { first: T, second: U }`)
	authority := seal(t, p)
	pair := alias(t, p, 0)
	if value, ok := authority.Materialize(selectorFor(t, authority, p, pair)); ok || value != nil {
		t.Fatalf("cyclic formal bounds materialized as %T without a lawful typ Mu binder", value)
	}
}

func TestAuthorityRejectsConflictingInheritedInterfaceRequirements(t *testing.T) {
	p := lower(t, `
interface Left
  id: string
end
interface Right
  id: number
end
interface Conflict: Left, Right
end
`)
	authority := seal(t, p)
	conflict, ok := p.Static().Declarations().Interfaces().At(2)
	if !ok {
		t.Fatal("missing Conflict interface")
	}
	if value, ok := authority.Materialize(selectorFor(t, authority, p, conflict)); ok || value != nil {
		t.Fatalf("conflicting inherited members materialized as %T", value)
	}
}

func TestAuthorityAllowsIdenticalInheritedInterfaceRequirements(t *testing.T) {
	p := lower(t, `
interface Left
  id: string
end
interface Right
  id: string
end
interface Joined: Left, Right
end
`)
	authority := seal(t, p)
	joined, ok := p.Static().Declarations().Interfaces().At(2)
	if !ok {
		t.Fatal("missing Joined interface")
	}
	value, ok := authority.Materialize(selectorFor(t, authority, p, joined))
	if !ok {
		t.Fatal("identical inherited requirements did not materialize")
	}
	if field, ok := access.Field(value, "id"); !ok || field != typ.String {
		t.Fatalf("Joined.id=%v/%v, want string", field, ok)
	}
}

func TestAuthorityRejectsSemanticallyDuplicateDirectUnionArms(t *testing.T) {
	p := lower(t, `
type Duplicate = { tag: "left" } | { tag: "left" } | { tag: "right" }
`)
	authority := seal(t, p)
	declaration := alias(t, p, 0)
	_, root, _, _, ok := p.Static().Declarations().Aliases().Get(declaration)
	if !ok {
		t.Fatal("missing Duplicate target")
	}
	if family, ok := authority.Family(selectorFor(t, authority, p, root)); ok || family.Valid() {
		t.Fatalf("duplicate union arms formed family %#v", family)
	}
}

func TestAuthorityFamilyRequiresEveryArmPairToSeparate(t *testing.T) {
	p := lower(t, `
type Partial = { tag: "a" } | { tag: "b" } | { tag: "a", extra: number }
type Complete = { tag: "a" } | { tag: "b" } | { tag: "c" }
`)
	authority := seal(t, p)
	for _, want := range []struct {
		index int
		ok    bool
		arity int
	}{
		{index: 0, ok: false},
		{index: 1, ok: true, arity: 3},
	} {
		declaration := alias(t, p, want.index)
		_, root, _, _, ok := p.Static().Declarations().Aliases().Get(declaration)
		if !ok {
			t.Fatalf("missing target %d", want.index)
		}
		family, got := authority.Family(selectorFor(t, authority, p, root))
		if got != want.ok {
			t.Fatalf("family %d admitted=%v, want %v", want.index, got, want.ok)
		}
		if got && authority.FamilyArity(family) != want.arity {
			t.Fatalf("family %d arity=%d, want %d", want.index, authority.FamilyArity(family), want.arity)
		}
	}
}

func lower(t testing.TB, source string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "authority.lua", Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func seal(t testing.TB, p *program.Program) *typeauthority.Authority {
	t.Helper()
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "authority", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	authority, ok := typeauthority.Seal(linked)
	if !ok {
		t.Fatal("type authority did not seal")
	}
	return authority
}

func alias(t testing.TB, p *program.Program, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Static().Declarations().Aliases().At(index)
	if !ok {
		t.Fatalf("missing alias %d", index)
	}
	return term
}

func selectorFor(t testing.TB, authority *typeauthority.Authority, p *program.Program, root keyspace.Term) typeauthority.Selector {
	t.Helper()
	selector, ok := authority.Find(p.ContentID(), root)
	if !ok {
		t.Fatalf("missing selector for %v", root)
	}
	return selector
}
