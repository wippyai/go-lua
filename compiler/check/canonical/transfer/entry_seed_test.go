package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestEntrySeedRefinesDeclaredAnyArrayWithExactEntry(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	got := entrySeedValue(
		typ.NewArray(typ.Any),
		product.FromType(typ.NewArray(entry)),
		product.AbstractValue{},
	)
	want := typ.NewArray(entry)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("entry seed = %v, want %v", got.ProjectValue(), want)
	}
}

func TestEntrySeedKeepsBareDeclaredAnyAsDynamicTop(t *testing.T) {
	got := entrySeedValue(
		typ.Any,
		product.FromType(typ.NewRecord().Field("id", typ.String).Build()),
		product.AbstractValue{},
	)
	if !typ.TypeEquals(got.ProjectValue(), typ.Any) {
		t.Fatalf("bare any seed = %v, want any", got.ProjectValue())
	}
}

func TestEntrySeedBodyContractRefinesDeclaredAny(t *testing.T) {
	contract := typ.NewRecord().ReadonlyField("id", typ.String).Build()
	got := entrySeedValue(
		typ.Any,
		product.AbstractValue{},
		product.FromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), contract) {
		t.Fatalf("contract seed = %v, want %v", got.ProjectValue(), contract)
	}
}

func TestEntrySeedBodyContractRefinesEntryArrayElement(t *testing.T) {
	entry := typ.NewArray(typ.Any)
	contract := typ.NewArray(typ.NewRecord().ReadonlyField("id", typ.String).Build())
	got := entrySeedValue(
		typ.NewArray(typ.Any),
		product.FromType(entry),
		product.FromType(contract),
	)
	if !typ.TypeEquals(got.ProjectValue(), contract) {
		t.Fatalf("contract array seed = %v, want %v", got.ProjectValue(), contract)
	}
}

func TestEntrySeedEffectWritesRefinedDeclaredContainer(t *testing.T) {
	const sym = cfg.SymbolID(21)
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{}
	entry := typ.NewRecord().Field("id", typ.String).Build()

	tr.applyEntrySeedEffect(&out, EntrySeedEffect{
		Symbol:   sym,
		Declared: typ.NewArray(typ.Any),
		Entry:    product.FromType(typ.NewArray(entry)),
	})

	got, ok := tr.symbolValue(&out, sym)
	want := typ.NewArray(entry)
	if !ok || !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("seeded symbol = %v/%v, want %v", got.ProjectValue(), ok, want)
	}
}

func TestEntryReachabilityEffectLiftsBottomAxes(t *testing.T) {
	tr := New(input.Inputs{}, Config{})
	out := flow.PointState{
		Num:             numeric.Bottom(),
		CellEffects:     flow.CaptureEffectsDomain.Bottom(),
		ReceiverEffects: flow.ReceiverEffectsDomain.Bottom(),
	}

	if !tr.applyEntryReachabilityEffect(&out, EntryReachabilityEffect{}) {
		t.Fatal("entry reachability effect did not report a change")
	}
	if out.Num == nil || out.Num.IsUnsat() {
		t.Fatalf("entry numeric state = %v, want reachable empty state", out.Num)
	}
	if !constraint.Domain.Equal(out.Cond, constraint.Domain.Top()) {
		t.Fatalf("entry condition = %v, want reachable true condition", out.Cond)
	}
	if !flow.PointRelationsDomain.Equal(out.Rel, flow.PointRelationsDomain.Top()) {
		t.Fatalf("entry point relations = %#v, want reachable empty relation set", out.Rel)
	}
	if !flow.ReturnRelationsDomain.Equal(out.ReturnRel, flow.ReturnRelationsDomain.Top()) {
		t.Fatalf("entry return relations = %#v, want reachable empty relation set", out.ReturnRel)
	}
	if !flow.CaptureEffectsDomain.Equal(out.CellEffects, flow.CaptureEffectsIdentity()) {
		t.Fatalf("entry cell effects = %v, want identity", out.CellEffects)
	}
	if !flow.ReceiverEffectsDomain.Equal(out.ReceiverEffects, flow.ReceiverEffectsIdentity()) {
		t.Fatalf("entry receiver effects = %v, want identity", out.ReceiverEffects)
	}
	if !flow.StaticMemberFactsDomain.Equal(out.StaticMembers, flow.StaticMemberFactsDomain.Top()) {
		t.Fatalf("entry static member facts = %s, want reachable empty fact set", out.StaticMembers.Format())
	}
	if !flow.KeyPresenceFactsDomain.Equal(out.KeyPresence, flow.KeyPresenceFactsDomain.Top()) {
		t.Fatalf("entry key-presence facts = %s, want reachable empty fact set", out.KeyPresence.Format())
	}
	if !flow.ValueOriginFactsDomain.Equal(out.ValueOrigins, flow.ValueOriginFactsDomain.Top()) {
		t.Fatalf("entry value-origin facts = %s, want reachable empty fact set", out.ValueOrigins.Format())
	}
	if tr.applyEntryReachabilityEffect(&out, EntryReachabilityEffect{}) {
		t.Fatal("entry reachability effect should be idempotent")
	}
}

func TestLocalSoftContainerAnnotationDoesNotEraseKnownEmptyInitializer(t *testing.T) {
	const sym = cfg.SymbolID(31)
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: typ.NewArray(typ.Any)}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Symbol: sym, Name: "xs"}},
		Sources: []ast.Expr{&ast.TableExpr{}},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok {
		t.Fatal("soft container initializer did not write a value")
	}
	if typ.TypeEquals(got.ProjectValue(), typ.NewArray(typ.Any)) {
		t.Fatalf("soft container annotation erased known initializer: %v", got.ProjectValue())
	}
	entry := typ.NewRecord().Field("id", typ.String).Build()
	appended := product.AppendElement(got, product.FromType(entry))
	want := typ.NewArray(entry)
	if !typ.TypeEquals(appended.ProjectValue(), want) {
		t.Fatalf("append after soft initializer = %v, want %v", appended.ProjectValue(), want)
	}
}

func TestLocalConcreteContainerAnnotationStillSeedsUnknownInitializer(t *testing.T) {
	const sym = cfg.SymbolID(32)
	declared := typ.NewMap(typ.String, typ.String)
	tr := New(input.Inputs{}, Config{})
	tr.declaredTypes = map[cfg.SymbolID]typ.Type{sym: declared}

	out := flow.PointState{Env: map[flow.ValueKey]product.AbstractValue{}}
	tr.applyAssign(&out, 0, &cfg.AssignInfo{
		Targets: []cfg.AssignTarget{{Kind: cfg.TargetIdent, Symbol: sym, Name: "m"}},
		Sources: []ast.Expr{&ast.IdentExpr{Value: "unresolved"}},
	}, nil)

	got, ok := tr.symbolValue(&out, sym)
	if !ok || !typ.TypeEquals(got.ProjectValue(), declared) {
		t.Fatalf("concrete container declaration = %v/%v, want %v", got.ProjectValue(), ok, declared)
	}
}
