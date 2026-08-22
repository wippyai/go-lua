package oracle

import (
	"context"
	"strings"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
)

// The canonical fixture Target declares the time, assert2 and resource host
// modules, so a corpus source may require any of them and read the members it
// names. Admission is the declaration's whole authority: an operation the
// catalogue does not hold is a module the checker cannot judge, and the
// require-admission gate refuses the project rather than typing the absent
// surface as unknown.
func TestFixtureTargetHostModulesAdmitTheirDeclaredMembers(t *testing.T) {
	sealed := hostModuleTarget(t)
	for module, members := range map[string][]string{
		"time": {
			"now", "sleep", "timer", "after", "ticker", "date", "unix",
			"parse", "parse_duration", "load_location", "fixed_zone",
			"Time.sub", "Time.unix", "Time.format", "Time.add", "Time.round",
			"Duration.seconds", "Location.string", "Ticker.channel", "Timer.reset",
		},
		"assert2": {
			"eq", "neq", "ok", "fail", "is_nil", "not_nil", "is_string", "is_number",
			"is_table", "is_function", "is_boolean", "contains", "has_error", "no_error",
			"throws", "not_throws", "error_kind", "error_message", "error_contains",
		},
		"resource": {"connect", "close", "query", "begin", "commit"},
	} {
		for _, member := range members {
			t.Run(module+"."+member, func(t *testing.T) {
				if _, ok := sealed.Operations.Lookup(hostMemberBinding(module, member)); !ok {
					t.Fatalf("the sealed fixture Target holds no operation for %s.%s", module, member)
				}
			})
		}
	}
}

// A declared member answers the type the module actually produces. The clock
// surface is the case that matters to the corpus: an instant is a time.Time,
// the difference between two instants is a time.Duration, and a reading off
// either is the scalar the runtime returns.
func TestFixtureTargetTimeModuleAnswersItsDeclaredObjectTypes(t *testing.T) {
	sealed := hostModuleTarget(t)
	for _, law := range []struct {
		member string
		result int
		want   string
	}{
		{member: "now", result: 0, want: "time.Time"},
		{member: "Time.sub", result: 0, want: "time.Duration"},
		{member: "Time.location", result: 0, want: "time.Location"},
		{member: "Time.utc", result: 0, want: "time.Time"},
		{member: "after", result: 0, want: "time.Time"},
		{member: "Ticker.channel", result: 0, want: "time.Time"},
	} {
		t.Run("time."+law.member, func(t *testing.T) {
			declared := hostNormalResult(t, sealed, hostMemberBinding("time", law.member), 0, law.result)
			if !strings.Contains(declared.String(), law.want) {
				t.Fatalf("time.%s answers %s, want the declared %s", law.member, declared, law.want)
			}
		})
	}
}

// A fallible member's nil answer is its own normal arm. Declaring one arm that
// pairs the value with an optional error instead would publish the value as
// non-nil on the failure path, which is a proof the module never gave: v1
// answers nil alongside its error. The signature-derived throw arm survives the
// replacement, because these members also raise.
func TestFixtureTargetTimeFallibleMembersPublishSeparateOutcomeArms(t *testing.T) {
	sealed := hostModuleTarget(t)
	for _, member := range []string{"timer", "ticker", "parse", "parse_duration", "load_location"} {
		t.Run("time."+member, func(t *testing.T) {
			operation, ok := sealed.Operations.Lookup(hostMemberBinding("time", member))
			if !ok {
				t.Fatalf("the sealed fixture Target holds no operation for time.%s", member)
			}
			normals, throws := 0, 0
			success, failure := false, false
			for index := 0; index < sealed.Operations.OutcomeCount(operation); index++ {
				kind, _, outcomeOK := sealed.Operations.OutcomeAt(operation, index)
				if !outcomeOK {
					t.Fatalf("time.%s outcome %d is unavailable", member, index)
				}
				switch kind {
				case flowkind.OutcomeNormal:
					normals++
					value := hostNormalResult(t, sealed, hostMemberBinding("time", member), index, 0)
					reported := hostNormalResult(t, sealed, hostMemberBinding("time", member), index, 1)
					switch {
					case typ.TypeEquals(value, typ.Nil):
						if !strings.Contains(reported.String(), "Error") {
							t.Fatalf("time.%s answers nil alongside %s, want the module error type", member, reported)
						}
						failure = true
					case typ.TypeEquals(reported, typ.Nil):
						success = true
					default:
						t.Fatalf("time.%s normal arm %d answers (%s, %s), which is neither the success nor the failure arm",
							member, index, value, reported)
					}
				case flowkind.OutcomeThrow:
					throws++
				}
			}
			if normals != 2 || throws != 1 {
				t.Fatalf("time.%s publishes %d normal and %d throw arms, want 2 and 1", member, normals, throws)
			}
			if !success || !failure {
				t.Fatalf("time.%s publishes success=%t failure=%t arms, want both", member, success, failure)
			}
		})
	}
}

// assert2.fail always raises. A member declared to answer Never has no normal
// arm at all, so a caller reading past it holds no continuation the declaration
// did not give.
func TestFixtureTargetAssert2FailPublishesOnlyItsThrowArm(t *testing.T) {
	sealed := hostModuleTarget(t)
	operation, ok := sealed.Operations.Lookup(hostMemberBinding("assert2", "fail"))
	if !ok {
		t.Fatal("the sealed fixture Target holds no operation for assert2.fail")
	}
	normals, throws := 0, 0
	for index := 0; index < sealed.Operations.OutcomeCount(operation); index++ {
		kind, _, outcomeOK := sealed.Operations.OutcomeAt(operation, index)
		if !outcomeOK {
			t.Fatalf("assert2.fail outcome %d is unavailable", index)
		}
		switch kind {
		case flowkind.OutcomeNormal:
			normals++
		case flowkind.OutcomeThrow:
			throws++
		}
	}
	if normals != 0 || throws != 1 {
		t.Fatalf("assert2.fail publishes %d normal and %d throw arms, want 0 and 1", normals, throws)
	}
}

// A resource handle is a nominal declaration, not a table shape. close takes a
// connection and commit takes a transaction, so passing one where the other
// belongs is a contradiction the declaration can state.
func TestFixtureTargetResourceHandlesAreDistinctDeclaredTypes(t *testing.T) {
	sealed := hostModuleTarget(t)
	connection := hostNormalResult(t, sealed, hostMemberBinding("resource", "connect"), 0, 0)
	transaction := hostNormalResult(t, sealed, hostMemberBinding("resource", "begin"), 0, 0)
	if !strings.Contains(connection.String(), "resource.Connection") {
		t.Fatalf("resource.connect answers %s, want the declared resource.Connection", connection)
	}
	if !strings.Contains(transaction.String(), "resource.Transaction") {
		t.Fatalf("resource.begin answers %s, want the declared resource.Transaction", transaction)
	}
	if typ.TypeEquals(connection, transaction) {
		t.Fatal("resource.Connection and resource.Transaction seal to one type; the two lifecycles are then indistinguishable")
	}
	for _, law := range []struct {
		member string
		want   typ.Type
	}{{member: "close", want: connection}, {member: "query", want: connection}, {member: "commit", want: transaction}} {
		t.Run("resource."+law.member, func(t *testing.T) {
			operation, ok := sealed.Operations.Lookup(hostMemberBinding("resource", law.member))
			if !ok {
				t.Fatalf("the sealed fixture Target holds no operation for resource.%s", law.member)
			}
			input, inputOK := sealed.Operations.Input(operation)
			if !inputOK || sealed.Operations.ValuesCount(input) != 1 {
				t.Fatalf("resource.%s declares %d inputs, want the one handle it operates on", law.member, sealed.Operations.ValuesCount(input))
			}
			if declared := hostValueType(t, sealed, input, 0); !typ.TypeEquals(declared, law.want) {
				t.Fatalf("resource.%s takes %s, want %s", law.member, declared, law.want)
			}
		})
	}
}

// Every corpus fixture that requires a host module seals against the canonical
// fixture Target. A fixture that cannot link is not a partial analysis: the
// project never reaches the checker at all, so the whole judgment it was
// written to state goes unmeasured.
func TestCorpusHostModuleFixturesLinkAgainstCanonicalTarget(t *testing.T) {
	repository, err := testfixture.RepositoryRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := testfixture.LoadCorpus(repository)
	if err != nil {
		t.Fatal(err)
	}
	target := hostModuleTarget(t)
	for _, name := range []string{
		"flow/active-session-typed-map-time-sub",
		"flow/active-session-untyped-map-time-sub-soundness",
		"modules/active-session-any-time-sub-soundness",
		"modules/active-session-typed-time-sub",
		"modules/imported-map-of-time-record-store",
		"modules/imported-record-return-literal",
		"native/wippyv1-error-arm-nil-value",
		"native/wippyv1-expr-surface",
		"native/wippyv1-http-nil-arm",
		"native/wippyv1-http-surface",
		"native/wippyv1-json-surface",
		"native/wippyv1-process-nil-arm",
		"native/wippyv1-process-surface",
		"native/wippyv1-store-surface",
		"narrowing/partitioning/websocket-echo-select-payload",
		"realworld/agent-workflow-engine",
		"realworld/agent-workflow-engine-soundness",
		"realworld/cqrs-order-runtime",
		"realworld/cqrs-order-runtime-soundness",
		"realworld/event-bus-saga-runtime",
		"realworld/event-bus-saga-runtime-soundness",
		"realworld/middleware-session-router",
		"realworld/middleware-session-router-soundness",
		"realworld/notification-delivery-runtime",
		"realworld/notification-delivery-runtime-soundness",
		"realworld/plugin-runtime-pipeline",
		"realworld/plugin-runtime-pipeline-soundness",
		"realworld/plugin-supervisor-runtime",
		"realworld/plugin-supervisor-runtime-soundness",
		"realworld/tenant-policy-runtime",
		"realworld/tenant-policy-runtime-soundness",
		"realworld/transactional-saga-orchestrator",
		"realworld/transactional-saga-orchestrator-soundness",
		"semantic/declared-resource-lifecycle",
	} {
		t.Run(name, func(t *testing.T) {
			project, err := corpus.Project(name)
			if err != nil {
				t.Fatal(err)
			}
			linked, err := testfixture.SealCorpusProject(target, project)
			if err != nil {
				t.Fatal(err)
			}
			if linked.Project().Mounts().Count() != project.FileCount() {
				t.Fatalf("sealed mounts = %d, declared files = %d", linked.Project().Mounts().Count(), project.FileCount())
			}
		})
	}
}

func hostModuleTarget(t *testing.T) *contract.Contract {
	t.Helper()
	sealed, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

// hostMemberBinding names one mounted module member, including the members of a
// declared object type, which the catalogue splits on the same separator the
// manifest registered them under.
func hostMemberBinding(module, member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{module},
		Member:    strings.Split(member, "."),
	}
}

// hostNormalResult projects one result slot of one authored outcome back into a
// static type through the published Target query surface only.
func hostNormalResult(t *testing.T, sealed *contract.Contract, binding vocabulary.BindingSpec, outcome, result int) typ.Type {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(binding)
	if !ok {
		t.Fatalf("the sealed fixture Target holds no operation for %+v", binding)
	}
	_, values, outcomeOK := sealed.Operations.OutcomeAt(operation, outcome)
	if !outcomeOK {
		t.Fatalf("operation %+v outcome %d is unavailable", binding, outcome)
	}
	return hostValueType(t, sealed, values, result)
}

func hostValueType(t *testing.T, sealed *contract.Contract, values vocabulary.Values, index int) typ.Type {
	t.Helper()
	value, ok := sealed.Operations.ValuesAt(values, index)
	if !ok {
		t.Fatalf("value %d of the sealed values row is unavailable", index)
	}
	declaration, ok := sealed.Operations.TypeDeclaration(value)
	if !ok {
		t.Fatalf("value %d publishes no type declaration", index)
	}
	decoded, err := domaincontract.Decode(context.Background(), declaration, nil)
	if err != nil || decoded == nil {
		t.Fatalf("decode value %d: %v", index, err)
	}
	return decoded
}

// TestFixtureTargetMountsEveryWippyV1ReferenceMember states what mounting the
// reference half bought: every callable the v1 runtime declares for expr,
// http, json, process and store is an operation of the canonical fixture
// Target, reachable under the same manifest-local path the module registered
// it under. A member the composed Target drops is a member the corpus cannot
// call, and the fixture that calls it would measure the require-admission gate
// instead of the judgment it was written to state.
func TestFixtureTargetMountsEveryWippyV1ReferenceMember(t *testing.T) {
	sealed := hostModuleTarget(t)
	for _, module := range wippyv1.Modules() {
		declaration := module.Declaration()
		t.Run(module.Name, func(t *testing.T) {
			if len(declaration.FunctionSignatures) == 0 {
				t.Fatalf("the %s reference manifest declares no members", module.Name)
			}
			for member := range declaration.FunctionSignatures {
				if _, ok := sealed.Operations.Lookup(hostMemberBinding(module.Name, member)); !ok {
					t.Errorf("the sealed fixture Target holds no operation for %s.%s", module.Name, member)
				}
			}
		})
	}
}
