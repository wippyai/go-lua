package manifesttarget_test

import (
	"context"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/static"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// This is an adversarial probe on the host boundary, not a feature test. The
// attack is the cheapest unsoundness a checker can be given: let a host state
// that its function may return nothing, then take that statement away before
// the checker ever sees it.
//
// A host declaration is the only evidence the analyzer has about a native
// function. It never sees the body, so what the manifest states is what the
// checker treats as proven. A declared `T?` that arrives at the Target as `T`
// is therefore not a lost hint: it is a fabricated proof of non-nilness for a
// value the host is free to return as nil, and every consumer downstream -
// narrowing, guard elision, send-safety, the JIT's null-check removal - is
// entitled to rely on it.
//
// These probes assert what the boundary owes, never what it currently does.

// hostProbeNormalReturn projects one sealed host operation's first normal
// outcome value back into a static type. It reads only the published Target
// query surface, so a probe cannot pass by consulting a representation the
// checker does not consume.
func hostProbeNormalReturn(t *testing.T, sealed *contract.Contract, binding vocabulary.BindingSpec, index int) typ.Type {
	t.Helper()
	operation, operationOK := sealed.Operations.Lookup(binding)
	if !operationOK {
		t.Fatalf("sealed target has no operation for binding %+v", binding)
	}
	values, valuesOK := hostProbeNormalValues(t, sealed, operation)
	if !valuesOK {
		t.Fatalf("operation %+v publishes no normal outcome", binding)
	}
	if count := sealed.Operations.ValuesCount(values); count <= index {
		t.Fatalf("operation %+v normal outcome carries %d fixed values, want more than %d", binding, count, index)
	}
	valueType, valueTypeOK := sealed.Operations.ValuesAt(values, index)
	if !valueTypeOK {
		t.Fatalf("operation %+v normal outcome value %d unavailable", binding, index)
	}
	declaration, declarationOK := sealed.Operations.TypeDeclaration(valueType)
	if !declarationOK {
		t.Fatalf("operation %+v normal outcome value %d publishes no type declaration", binding, index)
	}
	decoded, err := domaincontract.Decode(context.Background(), declaration, nil)
	if err != nil || decoded == nil {
		t.Fatalf("decode operation %+v normal outcome value %d: %v", binding, index, err)
	}
	return decoded
}

// hostProbeNormalValues selects the operation's single normal outcome. An
// operation whose normal answer is split across alternatives states its
// nilability through those alternatives instead, so a probe that found more
// than one is measuring a different declaration than the one it attacks.
func hostProbeNormalValues(t *testing.T, sealed *contract.Contract, operation vocabulary.Operation) (vocabulary.Values, bool) {
	t.Helper()
	found := false
	var selected vocabulary.Values
	for index := 0; index < sealed.Operations.OutcomeCount(operation); index++ {
		kind, values, ok := sealed.Operations.OutcomeAt(operation, index)
		if !ok {
			t.Fatalf("operation outcome %d unavailable", index)
		}
		if kind != flowkind.OutcomeNormal {
			continue
		}
		if found {
			t.Fatalf("operation publishes more than one normal outcome; this probe measures the single-outcome declaration")
		}
		selected, found = values, true
	}
	return selected, found
}

// hostProbeAdmitsNil is the spelling-independent question the checker actually
// asks of a declared type: may a value of it carry the nil runtime family.
func hostProbeAdmitsNil(value typ.Type) bool {
	return static.MayRuntimeKinds(value)&runtimekind.Bit(runtimekind.Nil) != 0
}

// hostProbeSealed seals the standard providers plus one probe module.
func hostProbeSealed(t *testing.T, identity string, declare func(*manifestwire.Manifest)) *contract.Contract {
	t.Helper()
	providers := append(stdlib.Providers(), manifest.Provider{
		Identity: identity,
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("probe")
			declare(declaration)
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		t.Fatalf("seal probe catalogue: %v", err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatalf("seal probe target: %v", err)
	}
	return sealed
}

func hostProbeModuleBinding(member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"probe"}, Member: []string{member}}
}

// TestHostDeclaredOptionalReturnKeepsItsNilability is the probe. A host states
// that maybe_name may answer with nothing; the sealed Target must carry that
// statement, because the Target declaration is the whole of what the checker
// knows about this call.
//
// Losing it here is worse than losing an annotation in Lua source. A Lua body
// is re-derived from its own statements, so a dropped declaration is recovered.
// A host body is never analyzed: the manifest is the only evidence, so the
// stripped type is not an approximation of the truth, it is a claim the host
// never made, and it interns into the class table with the same standing as a
// derived proof.
func TestHostDeclaredOptionalReturnKeepsItsNilability(t *testing.T) {
	sealed := hostProbeSealed(t, "probe.optional-return", func(declaration *manifestwire.Manifest) {
		declaration.DefineFunctionSignature("maybe_name", signature.Function{
			Type: typ.Func().Param("key", typ.String).Returns(normalize.Optional(typ.String)).Build(),
		})
	})
	decoded := hostProbeNormalReturn(t, sealed, hostProbeModuleBinding("maybe_name"), 0)
	if !hostProbeAdmitsNil(decoded) {
		t.Fatalf("host declared maybe_name() -> string?; the sealed Target publishes %s, which admits %d and excludes nil, so every consumer reads a nil-free proof the host never gave",
			decoded, static.MayRuntimeKinds(decoded))
	}
}

// TestHostDeclaredOptionalParameterAndReturnAreTreatedAlike states the
// boundary's internal consistency. One declaration, one optional spelling, two
// positions: a parameter the host may be handed nothing for, and a result the
// host may answer nothing with. Nothing about the direction of a value changes
// whether nil is in its domain, so a boundary that preserves the optional in
// one position and drops it in the other is not applying a policy, it is
// losing information in one code path.
func TestHostDeclaredOptionalParameterAndReturnAreTreatedAlike(t *testing.T) {
	sealed := hostProbeSealed(t, "probe.optional-both", func(declaration *manifestwire.Manifest) {
		declaration.DefineFunctionSignature("echo", signature.Function{
			Type: typ.Func().Param("value", normalize.Optional(typ.String)).Returns(normalize.Optional(typ.String)).Build(),
		})
	})
	operation, operationOK := sealed.Operations.Lookup(hostProbeModuleBinding("echo"))
	if !operationOK {
		t.Fatal("sealed target has no probe.echo operation")
	}
	input, inputOK := sealed.Operations.Input(operation)
	if !inputOK {
		t.Fatal("probe.echo publishes no input values row")
	}
	parameterType, parameterTypeOK := sealed.Operations.ValuesAt(input, 0)
	if !parameterTypeOK {
		t.Fatal("probe.echo publishes no first parameter")
	}
	parameterDeclaration, parameterDeclarationOK := sealed.Operations.TypeDeclaration(parameterType)
	if !parameterDeclarationOK {
		t.Fatal("probe.echo first parameter publishes no type declaration")
	}
	parameter, err := domaincontract.Decode(context.Background(), parameterDeclaration, nil)
	if err != nil || parameter == nil {
		t.Fatalf("decode probe.echo parameter: %v", err)
	}
	result := hostProbeNormalReturn(t, sealed, hostProbeModuleBinding("echo"), 0)
	if hostProbeAdmitsNil(parameter) != hostProbeAdmitsNil(result) {
		t.Fatalf("one declaration, one optional spelling: parameter projects to %s (admits nil: %t) and result projects to %s (admits nil: %t)",
			parameter, hostProbeAdmitsNil(parameter), result, hostProbeAdmitsNil(result))
	}
}

// TestHostDeclaredNilUnionAndOptionalReturnAgree states that the boundary
// judges a type, not a spelling. `string?` and `string | nil` denote the same
// set of runtime values, and that set contains nil. Two obligations follow and
// both are stated here, because either alone can be met by a broken boundary:
// agreement alone is satisfied by discarding nil from both spellings, and
// checking one spelling alone leaves a fix that repairs `T?` and still loses
// an explicitly written nil member.
func TestHostDeclaredNilUnionAndOptionalReturnAgree(t *testing.T) {
	sealed := hostProbeSealed(t, "probe.spelling", func(declaration *manifestwire.Manifest) {
		declaration.DefineFunctionSignature("optional_spelling", signature.Function{
			Type: typ.Func().Returns(normalize.Optional(typ.String)).Build(),
		})
		declaration.DefineFunctionSignature("union_spelling", signature.Function{
			Type: typ.Func().Returns(typ.MaterializeUnion([]typ.Type{typ.String, typ.Nil})).Build(),
		})
	})
	optional := hostProbeNormalReturn(t, sealed, hostProbeModuleBinding("optional_spelling"), 0)
	union := hostProbeNormalReturn(t, sealed, hostProbeModuleBinding("union_spelling"), 0)
	if hostProbeAdmitsNil(optional) != hostProbeAdmitsNil(union) {
		t.Fatalf("the same declared return projects differently by spelling: string? -> %s (admits nil: %t), string | nil -> %s (admits nil: %t)",
			optional, hostProbeAdmitsNil(optional), union, hostProbeAdmitsNil(union))
	}
	if !hostProbeAdmitsNil(optional) || !hostProbeAdmitsNil(union) {
		t.Fatalf("both spellings name nil as a possible answer and both lose it: string? -> %s, string | nil -> %s", optional, union)
	}
}

// TestStandardLibraryOptionalReturnsKeepTheirNilability moves the same probe
// onto the library every analyzed program links against. These are not
// synthetic declarations: debug.setlocal answers nil when the level or index
// names no local, and debug.getinfo answers nil for a level past the stack.
// Each is declared `T?` in the manifest and carries no operational law that
// restates the nil answer as a separate outcome, so the sealed Target is the
// only place their nilability can survive.
func TestStandardLibraryOptionalReturnsKeepTheirNilability(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatalf("seal standard catalogue: %v", err)
	}
	sealed, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatalf("seal standard target: %v", err)
	}
	for _, probe := range []struct {
		module string
		member string
	}{
		{module: "debug", member: "setlocal"},
		{module: "debug", member: "getinfo"},
		{module: "debug", member: "setupvalue"},
	} {
		t.Run(probe.module+"."+probe.member, func(t *testing.T) {
			binding := vocabulary.BindingSpec{
				Namespace: vocabulary.BindingModule, Owner: []string{probe.module}, Member: []string{probe.member},
			}
			decoded := hostProbeNormalReturn(t, sealed, binding, 0)
			if !hostProbeAdmitsNil(decoded) {
				t.Fatalf("%s.%s is declared to return an optional; the sealed Target publishes %s, so a call result is read as never nil",
					probe.module, probe.member, decoded)
			}
		})
	}
}
