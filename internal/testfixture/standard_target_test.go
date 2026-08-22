package testfixture

import (
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/manifest"
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

// The resource module states both lifecycles as finite state machines, and the
// members that move a resource carry the transition they perform. Acquisition
// stays unstated because the manifest names a lifecycle subject with a
// parameter reference and both acquiring members produce their resource as a
// result; that gap belongs to the vocabulary, not to this declaration.
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
}
