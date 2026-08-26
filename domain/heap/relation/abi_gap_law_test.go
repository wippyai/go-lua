package relation_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	"github.com/wippyai/go-lua/domain/relationfixture"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The raw-access specimen.
//
// A binding lives outside the package whose mathematics it carries, so the
// only question that decides whether an operation can be bound is whether an
// outside caller can produce every operand the operation's signature demands.
// An exported name does not answer that question: a struct whose fields are
// typed by names its own package keeps to itself is exported and still
// unbuildable, and an enumeration the owner performs only through an
// unexported method is published in appearance alone.
//
// This specimen asks the question that decides it. It reads the operand types
// themselves rather than any description of them, so an operand that cannot be
// produced is a red law and never a passing note.

// standing is what one authored raw-access operation has: a binding, or a
// named reason it has none.
type standing uint8

const (
	// bound means one declared family carries this operation.
	bound standing = iota
	// operandUnreachable means the owner's judgment exists and at least one
	// operand its signature demands cannot be produced from outside the
	// owner's package.
	operandUnreachable
)

// operation is one semantic operation the two authored indexed raw-access
// plans state, the owner entry point that answers it, and where it stands.
type operation struct {
	plan  string
	stem  string
	entry string
	state standing
}

// rawAccess is the eleven operations the two indexed raw-access plans author.
func rawAccess() []operation {
	return []operation{
		{plan: "raw-get/key-routes", stem: "RawGetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-get/call-routes", stem: "RawGetCallRoutes", entry: "(*index.Topology).VisitReceiverCallDemand", state: bound},
		{plan: "raw-get/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-get/pack-routes", stem: "RawGetPackRoutes", entry: "(*index.Topology).VisitKeySelectors with (*index.Topology).VisitRoutePayloads", state: bound},
		{plan: "raw-get/source-routes", stem: "RawGetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-get/result", stem: "RawGetResult", entry: "(*index.Topology).RawGetReduce", state: operandUnreachable},
		{plan: "raw-set/key-routes", stem: "RawSetKeyRoutes", entry: "index.Index.DynamicKey with (*index.Topology).CoordinateName", state: bound},
		{plan: "raw-set/heap-routes", stem: "HeapReceiverRoutes", entry: "(*index.Topology).VisitReceiver", state: bound},
		{plan: "raw-set/pack-routes", stem: "RawSetPackRoutes", entry: "(*index.Topology).VisitKeySelectors with (*index.Topology).VisitRoutePayloads", state: bound},
		{plan: "raw-set/source-routes", stem: "RawSetSourceRoutes", entry: "(*index.Topology).VisitPayloadSources", state: bound},
		{plan: "raw-set/commit", stem: "RawSetCommit", entry: "(*index.Topology).RawSetMutateRoute", state: operandUnreachable},
	}
}

// owner is the package whose operands this specimen holds to being reachable.
const owner = "github.com/wippyai/go-lua/domain/heap/index"

// exportedName reports whether a declared name can be written by another
// package.
func exportedName(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper([]rune(name)[0])
}

// nameable reports whether a caller in another package can write this type
// down. A named type answers by its own name; an unnamed one answers by every
// type it is composed of, because a caller writes those instead.
func nameable(carried reflect.Type, seen map[reflect.Type]bool) (bool, string) {
	if carried == nil {
		return false, "a nil type"
	}
	if seen[carried] {
		return true, ""
	}
	seen[carried] = true

	if carried.PkgPath() != "" {
		name := carried.Name()
		if !exportedName(name) {
			return false, carried.PkgPath() + "." + name
		}
		return true, ""
	}
	switch carried.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
		return nameable(carried.Elem(), seen)
	case reflect.Map:
		if ok, reason := nameable(carried.Key(), seen); !ok {
			return false, reason
		}
		return nameable(carried.Elem(), seen)
	case reflect.Func:
		for index := 0; index < carried.NumIn(); index++ {
			if ok, reason := nameable(carried.In(index), seen); !ok {
				return false, reason
			}
		}
		for index := 0; index < carried.NumOut(); index++ {
			if ok, reason := nameable(carried.Out(index), seen); !ok {
				return false, reason
			}
		}
		return true, ""
	case reflect.Struct:
		for index := 0; index < carried.NumField(); index++ {
			if ok, reason := nameable(carried.Field(index).Type, seen); !ok {
				return false, reason
			}
		}
		return true, ""
	default:
		return true, ""
	}
}

// buildable reports whether a caller in another package can populate every
// field of one operand struct. Every field must be assignable by that caller
// and typed by something the caller can write.
func buildable(frame reflect.Type) []string {
	if frame == nil || frame.Kind() != reflect.Struct {
		return []string{"the operand is not a struct"}
	}
	refusals := make([]string, 0, frame.NumField())
	for index := 0; index < frame.NumField(); index++ {
		field := frame.Field(index)
		if field.PkgPath != "" {
			refusals = append(refusals, "field "+field.Name+" is unexported")
			continue
		}
		if ok, reason := nameable(field.Type, map[reflect.Type]bool{}); !ok {
			refusals = append(refusals, "field "+field.Name+" is typed by "+reason+", which its own package keeps to itself")
		}
	}
	return refusals
}

// TestEveryRawAccessFrameIsBuildableFromOutsideItsOwner is the operand law.
// A frame the owner exports but no one else can populate is not a published
// operand, and an operation that demands one cannot be bound however exported
// its own name is.
func TestEveryRawAccessFrameIsBuildableFromOutsideItsOwner(t *testing.T) {
	frames := []struct {
		label   string
		carried reflect.Type
	}{
		{label: "index.RawGetFrame", carried: reflect.TypeOf(indexdomain.RawGetFrame{})},
		{label: "index.RawSetFrame", carried: reflect.TypeOf(indexdomain.RawSetFrame{})},
	}
	for _, frame := range frames {
		refusals := buildable(frame.carried)
		for _, refusal := range refusals {
			t.Errorf("%s cannot be built by a caller outside %s: %s", frame.label, owner, refusal)
		}
	}
}

// TestTheOwnerPublishesTheSelectorsItsEnumerationDemands is the other half of
// the same question. VisitRoutePayloads reads the payloads a route carries
// under one key selector, so an outside caller must be able to obtain the
// selectors a candidate resolves to. The owner holds the sealed projection, so
// only the owner can answer, and it has to answer through something callable.
func TestTheOwnerPublishesTheSelectorsItsEnumerationDemands(t *testing.T) {
	selector := reflect.TypeOf(heapdomain.KeySelector{})
	topology := reflect.TypeOf(&indexdomain.Topology{})
	delivered := make([]string, 0, 2)
	for index := 0; index < topology.NumMethod(); index++ {
		method := topology.Method(index)
		if deliversSelector(method.Type, selector) {
			delivered = append(delivered, method.Name)
		}
	}
	if len(delivered) == 0 {
		t.Errorf("no exported method of *index.Topology delivers a %s to its caller, so the pack expansion cannot obtain the selector its enumeration reads", selector.String())
	}
}

// deliversSelector reports whether one method hands a selector to its caller,
// either by returning one or by visiting the caller with one.
func deliversSelector(method reflect.Type, selector reflect.Type) bool {
	for index := 0; index < method.NumOut(); index++ {
		if method.Out(index) == selector {
			return true
		}
	}
	for index := 0; index < method.NumIn(); index++ {
		argument := method.In(index)
		if argument.Kind() != reflect.Func {
			continue
		}
		for inner := 0; inner < argument.NumIn(); inner++ {
			if argument.In(inner) == selector {
				return true
			}
		}
	}
	return false
}

// TestAnOutsideCallerReallyBuildsBothFramesAndReachesTheSelectors is the
// concrete form of the two laws above. Reflection proves the shapes admit an
// outside caller; this builds them as one, against production authorities
// sealed from a real program, so what the specimen calls reachable is what a
// binding would actually write.
func TestAnOutsideCallerReallyBuildsBothFramesAndReachesTheSelectors(t *testing.T) {
	fixture := relationfixture.New(t)

	scratch, scratchOK := indexdomain.NewRawGetScratch(fixture.Topology)
	if !scratchOK || scratch == nil {
		t.Fatal("an outside caller could not open the reduction's reuse storage")
	}

	// Every selection slot of both frames is written here, by this package,
	// with values this package made. That is the whole property.
	absentValue := indexdomain.NewSelected(fixture.Values.Bottom(), false)
	get := indexdomain.RawGetFrame{
		Scratch:  scratch,
		Key:      absentValue,
		KeyCount: 1,
		Call: func(uint64) indexdomain.Selected[calldomain.Value] {
			return indexdomain.NewMissingSelected[calldomain.Value]()
		},
		Heap: func(heapdomain.RawRouteTag, heapdomain.Key) indexdomain.Selected[heapdomain.Value] {
			return indexdomain.NewMissingSelected[heapdomain.Value]()
		},
		Pack: func(heapdomain.RawPayloadTag) indexdomain.Selected[packdomain.Value] {
			return indexdomain.NewRefusedSelected[packdomain.Value]()
		},
		Source: func(indexdomain.RawSourceTag) indexdomain.Selected[valuedomain.Value] { return absentValue },
	}
	set := indexdomain.RawSetFrame{
		Key:      absentValue,
		KeyCount: 1,
		Pack: func(heapdomain.RawPayloadTag) indexdomain.Selected[packdomain.Value] {
			return indexdomain.NewRefusedSelected[packdomain.Value]()
		},
		Source: func(indexdomain.RawSourceTag) indexdomain.Selected[valuedomain.Value] { return absentValue },
	}
	if get.Call == nil || get.Heap == nil || get.Pack == nil || get.Source == nil || set.Pack == nil || set.Source == nil {
		t.Fatal("a frame this package wrote came back with an unwritten selection")
	}
	if !get.Key.Valid() || get.Key.Present() || !get.Key.Found() {
		t.Fatal("a delivered selection did not read back the disposition it was written with")
	}
	if indexdomain.NewMissingSelected[valuedomain.Value]().Found() {
		t.Fatal("a missing selection reported a row")
	}
	if indexdomain.NewRefusedSelected[valuedomain.Value]().Valid() {
		t.Fatal("a refused selection reported that it was read")
	}

	// The selector enumeration answers a caller outside the owner. The
	// fixture's candidates are not raw-access operands, so the enumeration
	// refuses them rather than answering; what this proves is that the call
	// is reachable and total, not that this fixture has a route to walk.
	if fixture.Topology.VisitKeySelectors(indexdomain.Index{}, absentValue, 1, func(heapdomain.KeySelector) bool { return true }) {
		t.Fatal("the selector enumeration answered for a candidate no topology issued")
	}
}

// TestEveryRawAccessEnumerationIsReachable drives the enumerations that are
// already published against production authorities sealed from a real program,
// so what this specimen calls reachable is what the code does.
func TestEveryRawAccessEnumerationIsReachable(t *testing.T) {
	fixture := relationfixture.New(t)
	routes := 0
	if !fixture.Topology.VisitReceiver(fixture.Receiver, nil, func(indexdomain.Route) bool {
		routes++
		return true
	}) {
		t.Fatal("the sealed topology refused the receiver its own program allocates")
	}
	if routes == 0 {
		t.Fatal("the sealed topology observed no route for a receiver that denotes a table root")
	}
	if !fixture.Topology.VisitReceiverCallDemand(fixture.Receiver, func(calldomain.Key, uint64) bool { return true }) {
		t.Fatal("the sealed topology refused the call demand of its own receiver")
	}
	named := 0
	for index := 0; index < fixture.Values.CoordinateCount(); index++ {
		coordinate, coordinateOK := fixture.Values.CoordinateAt(index)
		if !coordinateOK {
			t.Fatalf("coordinate %d is not issued", index)
		}
		if _, ok := fixture.Topology.CoordinateName(coordinate); ok {
			named++
		}
	}
	if named != fixture.Values.CoordinateCount() {
		t.Fatalf("the topology names %d of %d coordinates; a route destination must carry its owner's name", named, fixture.Values.CoordinateCount())
	}
}

// TestEveryUnboundRawAccessOperationIsCarriedAsANamedDebt states that what is
// still missing is carried rather than routed around, and that this specimen
// cannot award itself a binding: an operation it calls bound must be a family
// the corpus really declares and really emits.
func TestEveryUnboundRawAccessOperationIsCarriedAsANamedDebt(t *testing.T) {
	unbound := map[string]bool{}
	for _, entry := range rawAccess() {
		if entry.state == operandUnreachable {
			unbound[entry.stem] = true
		}
	}

	declared := 0
	pending := map[string]bool{}
	for _, family := range relbind.Declared().Families {
		if family.Census != "heap/index" {
			continue
		}
		declared++
		if family.Emitted() {
			continue
		}
		pending[family.Stem] = true
		if !strings.Contains(family.Pending, "w0-span-identity") {
			t.Errorf("family %s carries a debt that is not tagged with the operand that blocks it", family.Stem)
		}
		if strings.Contains(family.Pending, "unexported") {
			t.Errorf("family %s still blames an unexported owner symbol, and the owner publishes every operand its signatures name", family.Stem)
		}
	}
	if declared == 0 {
		t.Fatal("the corpus declares no indexed raw-access family")
	}
	for stem := range unbound {
		if !pending[stem] {
			t.Errorf("the specimen calls %s blocked and the corpus does not carry it as a debt", stem)
		}
	}
	for stem := range pending {
		if !unbound[stem] {
			t.Errorf("the corpus carries %s as a debt and the specimen does not say what blocks it", stem)
		}
	}

	emitted := map[string]bool{}
	for _, family := range relbind.Declared().Families {
		if family.Emitted() {
			emitted[family.Stem] = true
		}
	}
	for _, entry := range rawAccess() {
		if entry.state != bound {
			continue
		}
		if !emitted[entry.stem] {
			t.Errorf("%s is called bound and the corpus emits no family %s", entry.plan, entry.stem)
		}
	}
	blocked := make([]string, 0, len(unbound))
	for _, entry := range rawAccess() {
		if entry.state == operandUnreachable {
			blocked = append(blocked, entry.plan)
		}
	}
	t.Logf("raw-access operations authored: %d, bound: %d, blocked on an unreachable operand: %v", len(rawAccess()), len(rawAccess())-len(blocked), blocked)
}
