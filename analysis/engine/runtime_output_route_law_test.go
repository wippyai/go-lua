package engine

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/product"
	"github.com/wippyai/go-lua/analysis/identity"
)

// TestRouteOutputBatchRetainsNoDomainCallback prevents route publication from
// regaining a hidden second semantic phase after Routed returns. Lease is an
// engine scalar fence, not a callback or domain capability.
func TestRouteOutputBatchRetainsNoDomainCallback(t *testing.T) {
	typeOfBatch := reflect.TypeOf(routeOutputBatch[uint64]{})
	want := []string{"read", "selectionID", "lease", "refs", "values"}
	if typeOfBatch.NumField() != len(want) {
		t.Fatalf("route output batch fields = %d, want the one token plus paired vectors (%d)", typeOfBatch.NumField(), len(want))
	}
	for index := 0; index < typeOfBatch.NumField(); index++ {
		field := typeOfBatch.Field(index)
		if field.Name != want[index] {
			t.Fatalf("route output batch field %d = %q, want %q", index, field.Name, want[index])
		}
		if field.Type.Kind() == reflect.Func {
			t.Fatalf("route output batch retained callback field %q", field.Name)
		}
	}
}

type routeLeaseLawRef uint64

func (ref routeLeaseLawRef) factorRow() schemaFactorBinding { return nil }
func (ref routeLeaseLawRef) rawAddress() uint64             { return uint64(ref) }

type routeLeaseLawFixture struct {
	output *typedOutput[uint64, uint64]
	exec   *ruleExecution
	epoch  identity.Generation
	close  func()
}

func newRouteLeaseLawFixture(t testing.TB) routeLeaseLawFixture {
	t.Helper()
	fixture := newNewtonLawFixture(t, 1)
	work, ok := fixture.composition.NewWork()
	if !ok || work == nil {
		t.Fatal("route lease work")
	}
	epoch := identity.Generation(1)
	exec := &ruleExecution{work: work, epoch: epoch}
	exec.active.Open(epoch)
	rows, ok := product.NewRows(fixture.whole)
	if !ok {
		exec.active.Revoke(epoch)
		work.Close()
		t.Fatal("route lease rows")
	}
	exec.product = &productSession{
		execution: exec,
		work:      work,
		rows:      rows,
		reads:     []readRuntime{nil},
		values:    []provenanceRow{extendProvenance(provenanceRow{}, 0, 1)},
		live:      true,
		current:   0,
	}
	output := &typedOutput[uint64, uint64]{
		execution: exec,
		routeRead: 1,
		routeTarget: func(exactRef) (carrier.Target, schemaFactorBinding, uint64, bool) {
			return carrier.Target{}, nil, 0, false
		},
	}
	return routeLeaseLawFixture{
		output: output,
		exec:   exec,
		epoch:  epoch,
		close: func() {
			exec.product.close()
			exec.active.Revoke(epoch)
			work.Close()
		},
	}
}

func TestRouteOutputEmptySelectionDoesNotRequireLease(t *testing.T) {
	fixture := newRouteLeaseLawFixture(t)
	defer fixture.close()
	batch := routeOutputBatch[uint64]{read: 0, selectionID: 1}
	if !fixture.output.noSelection(fixture.exec, fixture.epoch, 0, batch) {
		t.Fatal("empty route selection refused without a batch lease")
	}
	if len(fixture.output.disposition) != 1 || fixture.output.disposition[0] != outputNoCandidate {
		t.Fatalf("empty route disposition = %v", fixture.output.disposition)
	}
	if fixture.output.noSelection(fixture.exec, fixture.epoch, 0, batch) {
		t.Fatal("empty route selection settled twice")
	}
}

func TestRouteOutputLeaseFencesLifetimeAndWrongRowRelease(t *testing.T) {
	fixture := newRouteLeaseLawFixture(t)
	defer fixture.close()
	refs, values, lease, ok := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, 2)
	if !ok || lease == 0 || len(refs) != 2 || len(values) != 2 {
		t.Fatal("route lease reservation")
	}
	refs[0], values[0] = routeLeaseLawRef(1), 42
	batch := routeOutputBatch[uint64]{read: 0, selectionID: 1, lease: lease, refs: refs, values: values}
	if fixture.output.releaseRoute(fixture.exec, fixture.epoch, 1, 0, 1, lease) {
		t.Fatal("wrong-row route release succeeded")
	}
	if !fixture.output.routeBusy || values[0] != 42 || refs[0] == nil {
		t.Fatal("wrong-row release mutated the live lease")
	}
	if !fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, lease) || fixture.output.routeBusy {
		t.Fatal("route lease did not release with its exact fence")
	}
	if values[0] != 0 || refs[0] != nil {
		t.Fatal("released route scratch retained an aliased value")
	}
	if fixture.output.validRouteBatch(fixture.exec, fixture.epoch, 0, batch) {
		t.Fatal("released route batch remained valid")
	}

	nextRefs, nextValues, nextLease, nextOK := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, 2)
	if !nextOK || nextLease == lease || &nextRefs[0] != &refs[0] || &nextValues[0] != &values[0] {
		t.Fatal("route scratch did not warm-reuse its backing arrays")
	}
	if fixture.output.validRouteBatch(fixture.exec, fixture.epoch, 0, batch) {
		t.Fatal("stale same-sized route batch crossed the lease fence")
	}
	nextBatch := routeOutputBatch[uint64]{read: 0, selectionID: 1, lease: nextLease, refs: nextRefs, values: nextValues}
	if !fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, nextLease) {
		t.Fatal("second route lease release")
	}
	thirdRefs, thirdValues, thirdLease, thirdOK := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, 2)
	if !thirdOK || thirdLease == nextLease || &thirdRefs[0] != &nextRefs[0] || &thirdValues[0] != &nextValues[0] {
		t.Fatal("third route lease reservation")
	}
	if fixture.output.validRouteBatch(fixture.exec, fixture.epoch, 0, batch) || fixture.output.validRouteBatch(fixture.exec, fixture.epoch, 0, nextBatch) {
		t.Fatal("two-generation stale route batch crossed the lease fence")
	}
	fixture.exec.active.Revoke(fixture.epoch)
	if fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, thirdLease) {
		t.Fatal("revoked epoch released a route lease")
	}
	if !fixture.output.routeBusy {
		t.Fatal("revoked epoch release mutated the live lease")
	}
	fixture.output.clearRouteReservation()
}

func TestRouteOutputBusyRejectsDirectAndEmptyOutcomes(t *testing.T) {
	fixture := newRouteLeaseLawFixture(t)
	defer fixture.close()
	_, _, lease, ok := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, 1)
	if !ok || lease == 0 {
		t.Fatal("route lease reservation")
	}
	if fixture.output.stage(fixture.exec, fixture.epoch, 0, 1) || fixture.output.noCandidate(fixture.exec, fixture.epoch, 0) || fixture.output.noSelection(fixture.exec, fixture.epoch, 0, routeOutputBatch[uint64]{read: 0, selectionID: 1}) {
		t.Fatal("route-busy output accepted a competing disposition")
	}
	if !fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, lease) {
		t.Fatal("route lease release")
	}
}

func TestRouteOutputGroupedValuesRemainOrderedContiguousSubSlices(t *testing.T) {
	issuer, ok := carrier.NewIssuer()
	if !ok {
		t.Fatal("route group issuer")
	}
	first, firstOK := issuer.IssueTarget(1, carrier.StrongTarget)
	second, secondOK := issuer.IssueTarget(2, carrier.StrongTarget)
	if !firstOK || !secondOK {
		t.Fatal("route group targets")
	}
	routes := []resolvedRuleTarget{{target: first}, {target: first}, {target: second}, {target: second}}
	values := []uint64{1, 2, 3, 4}
	var joined []uint64
	var starts []int
	if !forEachRouteGroup(routes, values, func(target carrier.Target, group []uint64) bool {
		if target.Same(first) && &group[0] != &values[0] || target.Same(second) && &group[0] != &values[2] {
			t.Fatal("group was copied instead of passed as a contiguous subslice")
		}
		starts = append(starts, int(group[0]))
		var fold uint64
		for _, value := range group {
			fold = fold*10 + value
		}
		joined = append(joined, fold)
		return true
	}) || !reflect.DeepEqual(starts, []int{1, 3}) || !reflect.DeepEqual(joined, []uint64{12, 34}) {
		t.Fatalf("group order starts=%v joined=%v", starts, joined)
	}
}

type routeWarmReuseCase struct {
	fixture  routeLeaseLawFixture
	width    int
	refs     []exactRef
	refStore []routeLeaseLawRef
	routes   []resolvedRuleTarget
	state    *routeWarmJoinState
	visit    func(carrier.Target, []uint64) bool
}

type routeWarmJoinState struct {
	groups int
	joined uint64
}

func newRouteWarmReuseCase(t testing.TB, width int) routeWarmReuseCase {
	t.Helper()
	if width <= 0 {
		t.Fatal("route warm width")
	}
	fixture := newRouteLeaseLawFixture(t)
	issuer, ok := carrier.NewIssuer()
	if !ok {
		fixture.close()
		t.Fatal("route warm issuer")
	}
	refStore := make([]routeLeaseLawRef, width)
	refs := make([]exactRef, width)
	routes := make([]resolvedRuleTarget, width)
	for index := 0; index < width; index++ {
		target, targetOK := issuer.IssueTarget(uint64(index+1), carrier.StrongTarget)
		if !targetOK {
			fixture.close()
			t.Fatal("route warm target")
		}
		refStore[index] = routeLeaseLawRef(index)
		refs[index] = &refStore[index]
		routes[index] = resolvedRuleTarget{target: target}
	}
	state := &routeWarmJoinState{}
	result := routeWarmReuseCase{fixture: fixture, width: width, refs: refs, refStore: refStore, routes: routes, state: state}
	result.visit = func(_ carrier.Target, values []uint64) bool {
		state.groups++
		for _, value := range values {
			state.joined += value
		}
		return true
	}
	return result
}

func (fixture *routeWarmReuseCase) run() bool {
	if fixture == nil || fixture.width <= 0 || len(fixture.refs) != fixture.width || len(fixture.routes) != fixture.width || fixture.state == nil || fixture.visit == nil {
		return false
	}
	fixture.state.groups, fixture.state.joined = 0, 0
	refs, values, lease, ok := fixture.fixture.output.reserveRoute(fixture.fixture.exec, fixture.fixture.epoch, 0, 0, 1, fixture.width)
	if !ok || lease == 0 || len(refs) != fixture.width || len(values) != fixture.width {
		return false
	}
	copy(refs, fixture.refs)
	for index := range values {
		values[index] = uint64(index + 1)
	}
	fixture.fixture.output.routeTargets = fixture.fixture.output.routeTargets[:fixture.width]
	copy(fixture.fixture.output.routeTargets, fixture.routes)
	batch := routeOutputBatch[uint64]{read: 0, selectionID: 1, lease: lease, refs: refs, values: values}
	valid := fixture.fixture.output.validRouteBatch(fixture.fixture.exec, fixture.fixture.epoch, 0, batch)
	grouped := valid && forEachRouteGroup(fixture.fixture.output.routeTargets, values, fixture.visit)
	released := fixture.fixture.output.releaseRoute(fixture.fixture.exec, fixture.fixture.epoch, 0, 0, 1, lease)
	return grouped && released && !fixture.fixture.output.routeBusy && fixture.state.groups == fixture.width && fixture.state.joined == uint64(fixture.width*(fixture.width+1)/2)
}

func TestRouteOutputWarmExactAndLazyAllRootPathsAllocateZero(t *testing.T) {
	for _, test := range []struct {
		name  string
		width int
	}{
		{name: "exact", width: 1},
		{name: "lazy-all-root", width: 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRouteWarmReuseCase(t, test.width)
			defer fixture.fixture.close()
			if !fixture.run() || !fixture.run() {
				t.Fatal("route warm path")
			}
			allocations := testing.AllocsPerRun(100, func() {
				if !fixture.run() {
					t.Fatal("route warm path iteration")
				}
			})
			if allocations != 0 {
				t.Fatalf("route warm path allocations = %v, want zero", allocations)
			}
		})
	}
}

func BenchmarkTypedOutputRouteWarmExactAndLazyAllRoot(b *testing.B) {
	for _, test := range []struct {
		name  string
		width int
	}{
		{name: "exact", width: 1},
		{name: "lazy-all-root", width: 1024},
	} {
		test := test
		b.Run(test.name, func(b *testing.B) {
			fixture := newRouteWarmReuseCase(b, test.width)
			defer fixture.fixture.close()
			if !fixture.run() {
				b.Fatal("route warm path")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if !fixture.run() {
					b.Fatal("route warm path iteration")
				}
			}
		})
	}
}

func BenchmarkTypedOutputRouteLeaseWarmReuse(b *testing.B) {
	for _, width := range []int{1, 2, 8, 16, 64, 1024} {
		b.Run(fmt.Sprintf("width=%d", width), func(b *testing.B) {
			fixture := newRouteLeaseLawFixture(b)
			defer fixture.close()
			if _, _, lease, ok := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, width); !ok || !fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, lease) {
				b.Fatal("route lease warm-up")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				_, values, lease, ok := fixture.output.reserveRoute(fixture.exec, fixture.epoch, 0, 0, 1, width)
				if !ok || lease == 0 {
					b.Fatal("route lease reservation")
				}
				for index := range values {
					values[index] = uint64(index)
				}
				if !fixture.output.releaseRoute(fixture.exec, fixture.epoch, 0, 0, 1, lease) {
					b.Fatal("route lease release")
				}
			}
		})
	}
}
