package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestPublicationEscapeProgramIsEffectCallValuePlacementJWR(t *testing.T) {
	declaration := PublicationEscape()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("publication-escape declaration rejected: %+v", problem)
	}
	if declaration.OperandRole != "semantic/operand/placement/publication-escape" {
		t.Fatalf("operand role = %q", declaration.OperandRole)
	}
	if declaration.Candidate.AxisRelation.Axis.Key != effectAxisKey || declaration.Candidate.AxisRelation.Member != EffectMountedCallCandidates {
		t.Fatalf("candidate = %+v, want Effect's canonical mounted-call directory", declaration.Candidate)
	}
	if got := declaration.JoinCount(); got != 3 {
		t.Fatalf("join count = %d, want Call, Value, Placement", got)
	}

	call, callOK := declaration.JoinAt(0)
	if !callOK || call.Read.Form != ruleprogram.Exact || call.Read.Axis.EntryReference().Key != callAxisKey ||
		call.Relation.Member != CallMountedFacts || call.Key.Member != CallMountedFactKey ||
		len(call.Sources) != 1 || !call.Sources[0].Candidate || call.Predicate.Declared() {
		t.Fatalf("Call join = %+v, want one batch-gated exact read", call)
	}

	sources, sourcesOK := declaration.JoinAt(1)
	if !sourcesOK || sources.Read.Form != ruleprogram.Selected || sources.Read.Axis.EntryReference().Key != valueAxisKey ||
		sources.Relation.Axis.Key != effectAxisKey || sources.Key.Axis.Key != effectAxisKey || sources.Predicate.Axis.Key != effectAxisKey ||
		sources.Relation.Member != PublicationSources || sources.Key.Member != PublicationSourceKey ||
		sources.Predicate.Member != PublicationSourceTag || len(sources.Sources) != 2 ||
		!sources.Sources[0].Candidate || sources.Sources[1] != ruleprogram.PriorSource(0) {
		t.Fatalf("Value source join = %+v, want batch plus Call fact", sources)
	}
	if sources.Read.Contract.DenominatorRef.EntryReference().Key != schema.Key("coordinates/value") {
		t.Fatalf("Value denominator = %+v, want coordinates/value", sources.Read.Contract.DenominatorRef)
	}

	routes, routesOK := declaration.JoinAt(2)
	if !routesOK || routes.Read.Form != ruleprogram.Selected || routes.Read.Axis.EntryReference().Key != AxisKey ||
		routes.Relation.Member != PublicationRoutes || routes.Key.Member != PublicationRouteKey || routes.Predicate.Declared() ||
		len(routes.Sources) != 3 || !routes.Sources[0].Candidate || routes.Sources[1] != ruleprogram.PriorSource(0) || routes.Sources[2] != ruleprogram.PriorSource(1) {
		t.Fatalf("Placement route join = %+v, want untagged RouteMember selected read", routes)
	}
	if routes.Read.Contract.DenominatorRef.EntryReference().Key != schema.Key("coordinates/placement") {
		t.Fatalf("Placement denominator = %+v, want coordinates/placement", routes.Read.Contract.DenominatorRef)
	}

	if len(declaration.Fold.Inputs) != 1 || declaration.Fold.Inputs[0] != 2 {
		t.Fatalf("fold inputs = %v, want only the authenticated routed Placement read", declaration.Fold.Inputs)
	}
	if len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("fold outputs = %d, want one routed Placement output", len(declaration.Fold.Outputs))
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 ||
		output.Column.Key != OutputKey || output.Destination.Member != PublicationRouteDest {
		t.Fatalf("route output = %+v, want explicit deferred destination on Join 2", output)
	}
	if declaration.Carry == nil || declaration.Carry.Input != 0 || declaration.Carry.Mode != ruleprogram.CarryIdentity || declaration.Carry.Transform.Declared() {
		t.Fatalf("carry = %+v, want identity carry on input 0", declaration.Carry)
	}
}

func TestPublicationEscapeProgramUsesOneStrictExplicitReadContract(t *testing.T) {
	declaration := PublicationEscape()
	for index := 0; index < declaration.JoinCount(); index++ {
		join, ok := declaration.JoinAt(index)
		if !ok {
			t.Fatalf("join %d unavailable", index)
		}
		contract := join.Read.Contract
		if contract.Order != ruleprogram.OrderCanonical || contract.Sparse != ruleprogram.SparseExplicit ||
			contract.OnOpaque != ruleprogram.OnOpaqueRefuse || contract.Multiplicity != ruleprogram.MultiplicityOne {
			t.Fatalf("join %d contract = %+v, want strict canonical one-cell materialization", index, contract)
		}
	}
}

func TestPublicationEscapeProgramRefusesMissingRouteAndKeepsTheCanonicalReducer(t *testing.T) {
	missingRoute := PublicationEscape()
	missingRoute.Joins[2].Predicate.Axis = schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: AxisKey}
	missingRoute.Joins[2].Predicate.Member = "placement/publication-escape/route-tag"
	missingRoute.Fold.Outputs[0].RouteJoinPresent = false
	if problem, valid := missingRoute.Check(); valid || problem.Kind != ruleprogram.ProblemOutput {
		t.Fatalf("missing route join valid=%v problem=%+v", valid, problem)
	}

	wrongRoute := PublicationEscape()
	wrongRoute.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := wrongRoute.Check(); valid || problem.Kind != ruleprogram.ProblemJoin {
		t.Fatalf("non-route selected join valid=%v problem=%+v", valid, problem)
	}

	missingReducer := PublicationEscape()
	missingReducer.Fold.Reducer.Member = ""
	if problem, valid := missingReducer.Check(); valid || problem.Kind != ruleprogram.ProblemFold {
		t.Fatalf("missing reducer valid=%v problem=%+v", valid, problem)
	}
}

func TestPublicationEscapeRuleEntryCarriesFreshIssuanceAndProgram(t *testing.T) {
	spec := RuleEntry()
	if spec.Key != RuleKey || spec.Writes != AxisKey || spec.Owner != AxisKey ||
		spec.Semantic != "semantic/rule/placement/publication-escape" || len(spec.Roles) != 1 ||
		spec.Roles[0] != "semantic/operand/placement/publication-escape" {
		t.Fatalf("RuleEntry identity = %+v", spec)
	}
	if len(spec.Issues) != 1 || !spec.Issues[0].Available() {
		t.Fatalf("issuance = %+v, want one mounted call-effect issue", spec.Issues)
	}
	if problem, valid := spec.Program.Check(); !valid {
		t.Fatalf("RuleEntry program rejected: %+v", problem)
	}
	spec.Issues[0].Occurrence = "mutated"
	if RuleEntry().Issues[0].Occurrence == "mutated" {
		t.Fatal("issuance geometry is shared between declarations")
	}
}
