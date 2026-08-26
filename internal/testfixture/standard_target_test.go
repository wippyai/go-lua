package testfixture

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/type/access"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
)

func TestStandardLibraryTargetBindsChannelSelect(t *testing.T) {
	contract, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, ok := contract.InitialBinding(channelselect.ModuleName); !ok {
		t.Fatal("channel is not an initial global")
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{channelselect.ModuleName},
		Member:    []string{"select"},
	})
	if !ok {
		t.Fatal("channel.select is not a target operation")
	}
	_ = op
	member, status := typecall.MemberCall(channelselect.ModuleType(), "select")
	if status != typecall.MemberCallOK || !channelselect.IsSelectFunction(member) {
		t.Fatalf("MemberCall(ModuleType, select) = %v/%v", member, status)
	}
	if !typ.TypeEquals(member, channelselect.SelectFunction()) {
		t.Fatalf("select type = %v, want SelectFunction", member)
	}
	catalogue, err := manifest.Seal(manifest.Provider{
		Identity:    "testfixture.wippy.channel",
		Mount:       manifest.MountModule,
		Declaration: channelHostManifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := catalogue.Function("channel.select")
	if !ok || !channelselect.IsSelectFunction(fn.Signature().Type) {
		t.Fatalf("catalogue channel.select = %v/%v, want SelectFunction", fn.Signature().Type, ok)
	}
}

// stream is a host-style package: its module path is require-able, the same
// module table is present as the ambient stream binding, and only open is a
// callable member of that table.
func TestStandardLibraryTargetStreamHostSurface(t *testing.T) {
	target, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := target.InitialRootByModulePath("stream"); !ok {
		t.Fatal("stream is not a require-able initial module root")
	}
	if _, _, _, _, ok := target.InitialBinding("stream"); !ok {
		t.Fatal("stream is not an initial global binding")
	}
	open, ok := target.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"stream"},
		Member:    []string{"open"},
	})
	if !ok {
		t.Fatal("stream.open is not a target operation")
	}
	_, values, ok := target.Operations.OutcomeAt(open, 0)
	if !ok {
		t.Fatal("stream.open normal outcome is unavailable")
	}
	result, ok := target.Operations.ValuesAt(values, 0)
	if !ok {
		t.Fatal("stream.open result is unavailable")
	}
	declaration, ok := target.Operations.TypeDeclaration(result)
	if !ok {
		t.Fatal("stream.open result has no type declaration")
	}
	resultType, err := domaincontract.Decode(context.Background(), declaration, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := access.Field(resultType, "id")
	if !ok || !typ.TypeEquals(id, typ.String) {
		t.Fatalf("stream.open result id = %v/%v, want string", id, ok)
	}
	if _, ok := target.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"stream"},
		Member:    []string{"read"},
	}); ok {
		t.Fatal("stream.read is an undeclared nearest-neighbor operation")
	}
}

func TestStreamHostManifestDeclaresNamedExportAndOpenSignature(t *testing.T) {
	declaration := streamHostManifest()
	streamType, ok := declaration.Types["Stream"]
	if !ok || streamType == nil {
		t.Fatal("stream manifest does not declare Stream")
	}
	open, ok := declaration.FunctionSignatures["open"]
	if !ok || open.Type == nil || len(open.Type.Params) != 1 || len(open.Type.Returns) != 1 {
		t.Fatalf("stream.open signature = %v, want one string parameter and one result", open.Type)
	}
	if !typ.TypeEquals(open.Type.Params[0].Type, typ.String) || !typ.TypeEquals(open.Type.Returns[0], streamType) {
		t.Fatalf("stream.open signature = %v, want (string) -> Stream", open.Type)
	}
	export, ok := declaration.Export.(*typ.Record)
	if !ok {
		t.Fatalf("stream export = %T, want record", declaration.Export)
	}
	field := export.GetField("open")
	if field == nil || !typ.TypeEquals(field.Type, open.Type) {
		t.Fatalf("stream export open = %v, want %v", field, open.Type)
	}
	if export.GetField("read") != nil {
		t.Fatal("stream export declares nearest-negative member read")
	}
}

// The clock module names its object types, so a corpus source may annotate a
// value as time.Time and the annotation resolves to the same declaration the
// module's own members answer.
func TestTimeHostManifestNamesItsObjectTypes(t *testing.T) {
	declaration := timeHostManifest()
	for name, want := range map[string]string{
		"Time": "time.Time", "Duration": "time.Duration", "Location": "time.Location",
		"Ticker": "time.Ticker", "Timer": "time.Timer",
	} {
		declared := declaration.Types[name]
		if declared == nil {
			t.Fatalf("the time manifest declares no type %q", name)
		}
		if !strings.Contains(declared.String(), want) {
			t.Fatalf("time type %q is %s, want %s", name, declared, want)
		}
	}
}

// A duration argument admits exactly the three runtime forms the v1 coercion
// accepts. Declaring it as an unconstrained value instead would admit every
// argument the module rejects at runtime.
func TestTimeHostManifestAdmitsOnlyTheDurationFormsTheModuleAccepts(t *testing.T) {
	declaration := timeHostManifest()
	sleep, ok := declaration.FunctionSignatures["sleep"]
	if !ok || sleep.Type == nil || len(sleep.Type.Params) != 1 {
		t.Fatalf("the time manifest declares sleep as %v", sleep.Type)
	}
	admitted := sleep.Type.Params[0].Type
	union, isUnion := admitted.(*typ.Union)
	if !isUnion {
		t.Fatalf("time.sleep admits %s; the module accepts a Duration, a duration string, or a nanosecond count", admitted)
	}
	for _, form := range []typ.Type{declaration.Types["Duration"], typ.String, typ.Number} {
		admits := false
		for _, member := range union.Members {
			if typ.TypeEquals(member, form) {
				admits = true
			}
		}
		if !admits {
			t.Fatalf("time.sleep does not admit %s, which parseDurationValue accepts", form)
		}
	}
}

// assert2 answers its subject back. The library raises when the subject is nil,
// so the value the caller reads on the normal path is its own value with the
// nil case ruled out: the signature carries that as one type formal returned
// unchanged, and the refinement is what states the ruling out.
func TestAssert2HostManifestAnswersItsRefutedSubject(t *testing.T) {
	declaration := assert2HostManifest()
	for _, member := range []string{"ok", "not_nil"} {
		declared, ok := declaration.FunctionSignatures[member]
		if !ok || declared.Type == nil {
			t.Fatalf("the assert2 manifest declares no %s", member)
		}
		if len(declared.Type.TypeParams) != 1 || len(declared.Type.Returns) != 1 {
			t.Fatalf("assert2.%s declares %d type params and %d results, want one of each",
				member, len(declared.Type.TypeParams), len(declared.Type.Returns))
		}
		if !typ.TypeEquals(declared.Type.Returns[0], declared.Type.Params[0].Type) {
			t.Fatalf("assert2.%s answers %s for a subject of %s; the assertion returns the value it was given",
				member, declared.Type.Returns[0], declared.Type.Params[0].Type)
		}
		refuted := false
		for _, label := range declared.Effect.Labels {
			refinement, isRefinement := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
			if isRefinement && refinement.Target.Index == 0 && refinement.Refinement == (postcondition.Present{}) {
				refuted = true
			}
		}
		if !refuted {
			t.Fatalf("assert2.%s carries no presence refinement on its subject", member)
		}
	}
	failing, ok := declaration.FunctionSignatures["fail"]
	if !ok || failing.Type == nil || len(failing.Type.Returns) != 1 || !typ.TypeEquals(failing.Type.Returns[0], typ.Never) {
		t.Fatalf("assert2.fail is declared as %v, want a member that never returns", failing.Type)
	}
}

// The resource module states both lifecycles as finite state machines, the
// members that create a resource declare the result slot they acquire it in,
// and the members that move one carry the transition they perform. Every one
// of those declarations answers the sealed target's protocol query surface.
func TestResourceHostManifestDeclaresBothLifecycles(t *testing.T) {
	declaration := resourceHostManifest()
	for _, want := range []typestate.Definition{{
		Protocol:    "connection",
		States:      []typestate.State{"open", "closed"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}, {
		Protocol:    "transaction",
		States:      []typestate.State{"active", "committed"},
		FinalStates: []typestate.State{"committed"},
		Transitions: []typestate.TransitionDecl{{From: "active", To: "committed"}},
	}} {
		declared, ok := declaration.TypestateProtocol(want.Protocol)
		if !ok {
			t.Fatalf("the resource manifest declares no %q protocol", want.Protocol)
		}
		if !reflect.DeepEqual(declared.Normalized(), want.Normalized()) {
			t.Fatalf("protocol %q is %+v, want %+v", want.Protocol, declared, want)
		}
	}
	for member, want := range map[string]lifecycle.Transition{
		"close":  {Target: effect.ParamRef{Index: 0}, Protocol: "connection", From: "open", To: "closed"},
		"commit": {Target: effect.ParamRef{Index: 0}, Protocol: "transaction", From: "active", To: "committed"},
	} {
		declared, ok := declaration.FunctionSignatures[member]
		if !ok {
			t.Fatalf("the resource manifest declares no %s", member)
		}
		carried := false
		for _, label := range declared.Effect.Labels {
			if want.Equals(effect.NormalizeLabel(label)) {
				carried = true
			}
		}
		if !carried {
			t.Fatalf("resource.%s carries no %s", member, want)
		}
	}
	for member, want := range map[string]manifestwire.Acquisition{
		"connect": {Protocol: "connection", State: "open"},
		"begin":   {Protocol: "transaction", State: "active"},
	} {
		law, ok := declaration.FunctionOperations[member]
		if !ok {
			t.Fatalf("the resource manifest declares no operation law for %s", member)
		}
		if len(law.Acquisitions) != 1 || law.Acquisitions[0] != want {
			t.Fatalf("resource.%s acquisitions = %+v, want exactly %+v", member, law.Acquisitions, want)
		}
	}
	for member, want := range map[string]manifestwire.Requirement{
		"query": {Input: manifestwire.InputSource{Kind: manifestwire.InputSourceValue}, Protocol: "connection", State: "open"},
		"begin": {Input: manifestwire.InputSource{Kind: manifestwire.InputSourceValue}, Protocol: "connection", State: "open"},
	} {
		law, ok := declaration.FunctionOperations[member]
		if !ok {
			t.Fatalf("the resource manifest declares no operation law for %s", member)
		}
		if len(law.Requirements) != 1 || law.Requirements[0] != want {
			t.Fatalf("resource.%s requirements = %+v, want exactly %+v", member, law.Requirements, want)
		}
	}
	declared, declaredOK := declaration.FunctionSignatures["detach"]
	if !declaredOK {
		t.Fatal("the resource manifest declares no detach")
	}
	handed := lifecycle.Escape{Target: effect.ParamRef{Index: 0}, Protocol: "connection"}
	carried := false
	for _, label := range declared.Effect.Labels {
		if handed.Equals(effect.NormalizeLabel(label)) {
			carried = true
		}
	}
	if !carried {
		t.Fatalf("resource.detach carries no %s", handed)
	}
	assertSealedResourceLifecycles(t)
	assertSealedConnectionRequirements(t)
	assertSealedConnectionEscape(t)
}

// assertSealedConnectionEscape reads the authored escape back out of the
// sealed target. detach hands the connection out of the analysis, so the
// connection machine carries an escape row of its own beside the one the
// reader derives for every protocol's opaque operation, and that row names
// detach's first parameter.
func assertSealedConnectionEscape(t *testing.T) {
	t.Helper()
	sealed, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	table := sealed.Protocols()
	connection, _, found := protocolAcquiredBy(&table, resourceOperation(t, sealed, "connect"))
	if !found {
		t.Fatal("no sealed protocol is acquired by resource.connect")
	}
	detach := resourceOperation(t, sealed, "detach")
	authored := 0
	for index := 0; index < table.EscapeCount(connection); index++ {
		operation, kind, ordinal, ok := table.EscapeAt(connection, index)
		if !ok {
			t.Fatalf("connection escape %d is unavailable", index)
		}
		if operation != detach {
			continue
		}
		authored++
		if kind != vocabulary.InputSourceValueFormal || ordinal != 0 {
			t.Fatalf("resource.detach escapes %d/%d, want parameter 0", kind, ordinal)
		}
	}
	if authored != 1 {
		t.Fatalf("resource.detach owns %d sealed escape rows, want exactly one", authored)
	}
	for index := 0; index < table.TransitionCount(connection); index++ {
		moved, _, _, _, transitionOK := table.TransitionAt(connection, index)
		if transitionOK && moved == detach {
			t.Fatal("resource.detach moves the connection; an escape states no arrival")
		}
	}
}

// assertSealedConnectionRequirements reads the read-only constraints back out
// of the sealed target. Both members that read a connection without moving it
// answer the connection machine's open state on their first parameter, and
// neither of them moves it.
func assertSealedConnectionRequirements(t *testing.T) {
	t.Helper()
	sealed, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	table := sealed.Protocols()
	connection, _, found := protocolAcquiredBy(&table, resourceOperation(t, sealed, "connect"))
	if !found {
		t.Fatal("no sealed protocol is acquired by resource.connect")
	}
	if table.ProtocolRequirementCount(connection) != 2 {
		t.Fatalf("connection requirement count = %d, want the query and begin rows", table.ProtocolRequirementCount(connection))
	}
	for _, member := range []string{"query", "begin"} {
		operation := resourceOperation(t, sealed, member)
		var input vocabulary.InputSource
		var state vocabulary.State
		matches := 0
		for index := 0; index < table.DemandCount(operation); index++ {
			demand, ok := table.DemandAt(operation, index)
			if !ok {
				t.Fatalf("resource.%s demand %d is unavailable", member, index)
			}
			if demand.Kind != protocolvalue.DemandRequirement {
				continue
			}
			if demand.Protocol != connection {
				t.Fatalf("resource.%s constrains protocol %d, want the connection machine %d", member, demand.Protocol, connection)
			}
			_, declaredInput, declaredState, ok := table.ProtocolRequirementAt(demand.Protocol, demand.Row)
			if !ok {
				t.Fatalf("resource.%s requirement row %d is unavailable", member, demand.Row)
			}
			input, state = declaredInput, declaredState
			matches++
		}
		if matches != 1 {
			t.Fatalf("resource.%s requirement count = %d, want the single connection row", member, matches)
		}
		if input.Kind != vocabulary.InputSourceValueFormal || input.Ordinal != 0 {
			t.Fatalf("resource.%s constrains %+v, want parameter 0", member, input)
		}
		if name, ok := table.StateName(connection, state); !ok || name != "open" {
			t.Fatalf("resource.%s requires state %q/%v, want open", member, name, ok)
		}
		for index := 0; index < table.TransitionCount(connection); index++ {
			moved, _, _, _, transitionOK := table.TransitionAt(connection, index)
			if transitionOK && moved == operation {
				t.Fatalf("resource.%s moves the connection; a requirement declares no move", member)
			}
		}
	}
}

// assertSealedResourceLifecycles reads both declared machines back out of the
// sealed target. The sealed protocol carries no nominal protocol name - its
// identity is the set of result coordinates that create it - so each machine is
// found by the member that acquires it.
func assertSealedResourceLifecycles(t *testing.T) {
	t.Helper()
	sealed, err := StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	table := sealed.Protocols()
	for _, want := range []struct {
		acquire    string
		move       string
		from, to   string
		stateNames map[string]bool
	}{
		{acquire: "connect", move: "close", from: "open", to: "closed", stateNames: map[string]bool{"open": false, "closed": true}},
		{acquire: "begin", move: "commit", from: "active", to: "committed", stateNames: map[string]bool{"active": false, "committed": true}},
	} {
		acquirer := resourceOperation(t, sealed, want.acquire)
		mover := resourceOperation(t, sealed, want.move)
		protocol, state, found := protocolAcquiredBy(&table, acquirer)
		if !found {
			t.Fatalf("no sealed protocol is acquired by resource.%s", want.acquire)
		}
		if name, ok := table.StateName(protocol, state); !ok || name != want.from {
			t.Fatalf("resource.%s acquires state %q/%v, want %q", want.acquire, name, ok, want.from)
		}
		if got := sealedStateNames(&table, protocol); !reflect.DeepEqual(got, want.stateNames) {
			t.Fatalf("protocol acquired by resource.%s has states %v, want %v", want.acquire, got, want.stateNames)
		}
		if table.TransitionCount(protocol) != 1 {
			t.Fatalf("protocol acquired by resource.%s has %d transitions, want 1", want.acquire, table.TransitionCount(protocol))
		}
		operation, kind, ordinal, from, ok := table.TransitionAt(protocol, 0)
		if !ok || operation != mover || kind != vocabulary.InputSourceValueFormal || ordinal != 0 {
			t.Fatalf("transition subject = op %d %d/%d/%v, want resource.%s parameter 0", operation, kind, ordinal, ok, want.move)
		}
		if name, nameOK := table.StateName(protocol, from); !nameOK || name != want.from {
			t.Fatalf("transition source = %q/%v, want %q", name, nameOK, want.from)
		}
		if table.TransitionOutcomeCount(protocol, 0) != 1 {
			t.Fatalf("transition of resource.%s has %d outcome arms, want the normal arm alone", want.move, table.TransitionOutcomeCount(protocol, 0))
		}
		outcome, to, outcomeOK := table.TransitionOutcomeAt(protocol, 0, 0)
		if !outcomeOK || outcome != 0 {
			t.Fatalf("transition outcome = %d/%v, want the normal arm", outcome, outcomeOK)
		}
		if name, nameOK := table.StateName(protocol, to); !nameOK || name != want.to {
			t.Fatalf("transition target = %q/%v, want %q", name, nameOK, want.to)
		}
	}
}

func resourceOperation(t *testing.T, sealed *contract.Contract, member string) vocabulary.Operation {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule, Owner: []string{"resource"}, Member: []string{member},
	})
	if !ok {
		t.Fatalf("resource.%s is not a sealed operation", member)
	}
	return operation
}

func protocolAcquiredBy(table *protocolvalue.Table, operation vocabulary.Operation) (vocabulary.Protocol, vocabulary.State, bool) {
	for index := 0; index < table.ProtocolCount(); index++ {
		protocol, ok := table.ProtocolAt(index)
		if !ok {
			continue
		}
		for acquisition := 0; acquisition < table.ProtocolAcquisitionCount(protocol); acquisition++ {
			owner, _, _, state, found := table.ProtocolAcquisitionAt(protocol, acquisition)
			if found && owner == operation {
				return protocol, state, true
			}
		}
	}
	return 0, 0, false
}

func sealedStateNames(table *protocolvalue.Table, protocol vocabulary.Protocol) map[string]bool {
	out := make(map[string]bool, table.StateCount(protocol))
	for index := 0; index < table.StateCount(protocol); index++ {
		state, ok := table.StateAt(protocol, index)
		if !ok {
			continue
		}
		name, nameOK := table.StateName(protocol, state)
		final, finalOK := table.StateFinal(protocol, state)
		if !nameOK || !finalOK {
			continue
		}
		out[name] = final
	}
	return out
}
