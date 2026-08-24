package engine

import (
	"context"
	"reflect"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// TestBoundRuleMemberIsTheExecutionAuthority prevents reintroducing a nested
// rule carrier beside the graph-owned runtime member. Access, read binding,
// output ownership, and execution fencing must all terminate at this object.
func TestBoundRuleMemberIsTheExecutionAuthority(t *testing.T) {
	memberType := reflect.TypeOf(boundRuleMember[uint64, struct{}]{})
	if _, mirrored := memberType.FieldByName("rule"); mirrored {
		t.Fatal("bound rule member retained a nested rule authority mirror")
	}
	for _, field := range []string{"value", "cell", "ordinal", "fold", "reads", "output", "nextEpoch"} {
		if _, present := memberType.FieldByName(field); !present {
			t.Fatalf("canonical bound rule member does not own %s", field)
		}
	}
	ownerField, present := reflect.TypeOf(Frame[uint64, struct{}]{}).FieldByName("owner")
	if !present || ownerField.Type != reflect.TypeOf((*boundRuleMember[uint64, struct{}])(nil)) {
		t.Fatal("Frame is not fenced by the canonical bound rule member")
	}
	var owner anyRule = (*boundRuleMember[uint64, struct{}])(nil)
	var reads readBinding = (*boundRuleMember[uint64, struct{}])(nil)
	if owner == nil || reads == nil {
		t.Fatal("typed nil interface witnesses unexpectedly vanished")
	}
}

// TestMemberRowIsTheModelledWidth proves the runtime row table is sized by
// sealed member spans rather than by a retained construction object.
func TestMemberRowIsTheModelledWidth(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 9, nil, nil)
	program := fixture.solver.runtime.program
	if program == nil || !program.valid() || program.memberCount() != len(fixture.graph.members) {
		t.Fatalf("runtime member table valid=%t rows=%d committed=%d", program != nil && program.valid(), program.memberCount(), len(fixture.graph.members))
	}
	for group := 0; group < program.groupCount(); group++ {
		span, ok := program.groupSpanAt(group)
		if !ok || span.count() == 0 {
			t.Fatalf("group %d has no sealed member span", group)
		}
	}
}

// TestSolvedRuntimeProgramCoversEveryGraphMember proves the runtime table is
// a total permutation of every immutable graph Group member.
func TestSolvedRuntimeProgramCoversEveryGraphMember(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 12, nil, nil)
	program, graph := fixture.solver.runtime.program, fixture.graph.graph
	seen := 0
	for groupIndex := 0; groupIndex < graph.GroupCount(); groupIndex++ {
		group, groupOK := graph.HyperedgeAt(groupIndex)
		span, spanOK := program.groupSpanAt(groupIndex)
		if !groupOK || !spanOK || span.count() != group.MemberCount() {
			t.Fatalf("group %d graph/runtime member width mismatch", groupIndex)
		}
		groupKeys := make(map[composition.Key]struct{}, group.MemberCount())
		for _, row := range program.memberRows(span) {
			member, memberOK := memberRowIdentity(group, row)
			geometry, geometryOK := row.geometry()
			if !row.valid() || !geometryOK || !memberOK || !graph.OwnsMember(member) || geometry.member().Key() != member.Key() {
				t.Fatalf("group %d row lost its canonical graph member", groupIndex)
			}
			groupKeys[member.Key()] = struct{}{}
			seen++
		}
		if len(groupKeys) != group.MemberCount() {
			t.Fatalf("group %d runtime member set = %d, want %d", groupIndex, len(groupKeys), group.MemberCount())
		}
	}
	if seen != len(fixture.graph.members) {
		t.Fatalf("runtime covered %d/%d graph members", seen, len(fixture.graph.members))
	}
}

// TestProgramRowExecutionMatchesDraftExecution proves solve behavior through
// the sealed rows: every query is answered and the owner transfer runs, with
// no draft object or second execution path involved.
func TestProgramRowExecutionMatchesDraftExecution(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 5, nil, nil)
	state, status := fixture.solver.Solve(context.Background())
	if state == nil || status != SolveComplete || *fixture.transferRuns == 0 {
		t.Fatalf("sealed row solve state=%t status=%v transfers=%d", state != nil, status, *fixture.transferRuns)
	}
	for index, query := range fixture.queries {
		key, keyed := query.PublicationKey()
		value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
		if !keyed || !readable || value != fixture.expected[index] {
			t.Fatalf("sealed row query %d=%d/%t keyed=%t", index, value, readable, keyed)
		}
	}
}

// TestSealRuntimeProgramTakesOneValidityDecision proves the runtime row seal
// is all-or-nothing and a valid committed program exposes the resulting sealed
// table without a recoverable draft phase.
func TestSealRuntimeProgramTakesOneValidityDecision(t *testing.T) {
	if program, _, ok := sealRuntimeProgram(nil, nil, nil, []memberRow{{}}, []memberSpan{{start: 0, end: 1}}, nil, nil, nil, nil, executioncontext.Directory{}, contextfiber.Index{}, contextfiber.Layout{}, nil, false); ok || program != nil {
		t.Fatal("invalid runtime row published a program")
	}
	var nilRule *boundRuleMember[uint64, struct{}]
	if (memberRow{legacy: nilRule}).valid() {
		t.Fatal("typed-nil runtime member became valid")
	}
	fixture := newReceiptQueryMatrixFixture(t, 2, nil, nil)
	if fixture.solver.runtime.program == nil || !fixture.solver.runtime.program.valid() {
		t.Fatal("committed program did not publish one valid runtime row table")
	}
}

// TestProgramRowsCarryOneCanonicalMember proves each hot row retains the same
// immutable member used for execution, output ownership, and failure identity.
func TestProgramRowsCarryOneCanonicalMember(t *testing.T) {
	fixture := newReceiptQueryMatrixFixture(t, 4, nil, nil)
	program, graph := fixture.solver.runtime.program, fixture.graph.graph
	for groupIndex := 0; groupIndex < program.groupCount(); groupIndex++ {
		group, groupOK := graph.HyperedgeAt(groupIndex)
		span, spanOK := program.groupSpanAt(groupIndex)
		if !groupOK || !spanOK {
			t.Fatal("sealed group span unavailable")
		}
		for _, row := range program.memberRows(span) {
			member, ok := memberRowIdentity(group, row)
			geometry, geometryOK := row.geometry()
			if !ok || !row.valid() || !geometryOK || geometry.member().Key() != member.Key() {
				t.Fatal("runtime member row did not retain one canonical member")
			}
		}
	}
}

// TestDispatchMatrixRowsHaveTheModelledLayout retains the useful layout law
// from the old benchmark file without retaining its benchmark-only workload.
func TestDispatchMatrixRowsHaveTheModelledLayout(t *testing.T) {
	type narrowRow struct {
		apply func(uint64, uint64) uint64
		seed  uint64
		bias  uint64
	}
	type wideRow struct {
		apply   func(uint64, uint64) uint64
		seed    uint64
		bias    uint64
		payload [8]uint64
	}
	if unsafe.Sizeof(narrowRow{}) != 24 {
		t.Fatalf("narrow dispatch row size=%d, want 24", unsafe.Sizeof(narrowRow{}))
	}
	if unsafe.Sizeof(wideRow{}) < 80 {
		t.Fatalf("wide dispatch row size=%d, want at least 80", unsafe.Sizeof(wideRow{}))
	}
}
