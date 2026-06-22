package manifest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestManifestDefineTypeAndSetExport(t *testing.T) {
	m := New("example/module")
	user := typetable.NewRecord().
		Field("id", typ.Integer).
		OptField("name", typ.String).
		StaticStringIndex("role", typ.LiteralString("admin")).
		MapComponent(typ.String, typ.Number).
		Build()
	export := typ.Func().
		Param("user", user).
		Returns(typ.NewArray(user)).
		Build()

	m.DefineType("User", user)
	m.SetExport(export)

	if m.Path != "example/module" {
		t.Fatalf("path = %q", m.Path)
	}
	if got := m.Types["User"]; !typ.TypeEquals(got, user) {
		t.Fatalf("User type = %v, want %v", got, user)
	}
	if !typ.TypeEquals(m.Export, export) {
		t.Fatalf("export = %v, want %v", m.Export, export)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := New("example/module")
	m.Version = "v1"
	m.DefineType("Status", typeexpr.Union(
		typ.LiteralString("ready"),
		typ.LiteralString("pending"),
	))
	m.DefineType("User", typ.NewAnnotated(
		typetable.NewRecord().
			ReadonlyField("id", typ.Integer).
			OptField("name", typ.String).
			Build(),
		[]annotation.Annotation{{Name: "sealed", Arg: annotation.BoolArg(true)}},
	))
	m.SetExport(typ.Func().
		TypeParam("T", typ.NewRef("", "User")).
		Param("value", typ.NewTypeParam("T", typ.NewRef("", "User"))).
		Returns(typ.NewReadonlyMap(typ.String, typ.NewRef("", "Status"))).
		Build())

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got.Path != m.Path || got.Version != m.Version {
		t.Fatalf("metadata = %q/%q, want %q/%q", got.Path, got.Version, m.Path, m.Version)
	}
	if len(got.Types) != len(m.Types) {
		t.Fatalf("types = %d, want %d", len(got.Types), len(m.Types))
	}
	for name, want := range m.Types {
		if !typ.TypeEquals(got.Types[name], want) {
			t.Fatalf("type %s = %v, want %v", name, got.Types[name], want)
		}
	}
	if !typ.TypeEquals(got.Export, m.Export) {
		t.Fatalf("export = %v, want %v", got.Export, m.Export)
	}
}

func TestManifestRoundTripNormalizesMapKeysOnDecode(t *testing.T) {
	mapKey := typ.MaterializeUnion([]typ.Type{typ.String, typ.Nil})

	m := New("example/keys")
	m.DefineType("Writable", typ.NewMap(mapKey, typ.Number))
	m.DefineType("Readonly", typ.NewReadonlyMap(mapKey, typ.Number))

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	writable, ok := got.Types["Writable"].(*typ.Map)
	if !ok {
		t.Fatalf("Writable = %T, want *typ.Map", got.Types["Writable"])
	}
	if !typ.TypeEquals(writable.Key, typ.String) {
		t.Fatalf("Writable key = %v, want string", writable.Key)
	}

	readonly, ok := got.Types["Readonly"].(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("Readonly = %T, want *typ.ReadonlyMap", got.Types["Readonly"])
	}
	if !typ.TypeEquals(readonly.Key, typ.String) {
		t.Fatalf("Readonly key = %v, want string", readonly.Key)
	}
}

func TestManifestRejectsMalformedTypeWireMissingRequiredParts(t *testing.T) {
	tests := []struct {
		name string
		wire *typeWire
		want string
	}{
		{
			name: "map missing key",
			wire: &typeWire{Kind: "map", Value: &typeWire{Kind: "number"}},
			want: "map key missing type",
		},
		{
			name: "readonly map missing value",
			wire: &typeWire{Kind: "readonlyMap", Key: &typeWire{Kind: "string"}},
			want: "readonly map value missing type",
		},
		{
			name: "record map missing value",
			wire: &typeWire{Kind: "record", MapKey: &typeWire{Kind: "string"}},
			want: "record map value missing type",
		},
		{
			name: "annotation missing name",
			wire: &typeWire{
				Kind:        "annotated",
				Element:     &typeWire{Kind: "string"},
				Annotations: []annotationWire{{Kind: "nil"}},
			},
			want: "annotation missing name",
		},
		{
			name: "annotation missing kind",
			wire: &typeWire{
				Kind:        "annotated",
				Element:     &typeWire{Kind: "string"},
				Annotations: []annotationWire{{Name: "tag"}},
			},
			want: `annotation "tag" missing arg kind`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeType(tt.wire)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeType error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestRoundTripNamedFunctionSignatureEffects(t *testing.T) {
	row := effect.Open("rho",
		ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}},
		mutation.LengthChange{Target: effect.ParamRef{Index: 1}, Delta: -1},
		postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
		postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 1}, Refinement: postcondition.Absent{}},
		returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}},
	)
	export := typ.Func().
		Param("input", typ.String).
		Param("out", typ.NewArray(typ.String)).
		Returns(typ.String).
		Build()
	operational := &signature.OperationalEffects{
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{{
			TriggerIndex:    1,
			TriggerPresence: presence.Present(),
			TargetIndex:     0,
			TargetPresence:  presence.Absent(),
		}},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{{
			Path:     pathdom.NewPlaceholder(0).Field("ready"),
			Presence: presence.Present(),
		}},
		PathStaticMembers: []signature.PathStaticMemberFact{{
			Path: pathdom.NewPlaceholder(1).Field("kind"),
			Type: typ.String,
		}},
		PathInvalidations: []signature.PathInvalidation{{
			Path: pathdom.NewPlaceholder(1).Field("items"),
		}},
		FrozenTables: []signature.FrozenTable{{
			Target: pathdom.NewPlaceholder(0).Field("sealed"),
		}},
		EscapeEvents: []signature.EscapeEvent{{
			Target:    pathdom.NewPlaceholder(0).Field("payload"),
			Kind:      signature.EscapeSend,
			Recursive: true,
		}},
		StoreRelations: []signature.StoreRelation{{
			Source: pathdom.NewPlaceholder(0).Field("payload"),
			Into:   pathdom.NewPlaceholder(1).Field("items"),
		}},
		LifecycleEffects: []signature.LifecycleEffect{{
			Target:   pathdom.NewPlaceholder(0).Field("tx"),
			Kind:     signature.LifecycleAcquire,
			Protocol: typestate.Protocol("transaction"),
			To:       typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}, {
			Target:   pathdom.NewPlaceholder(0).Field("tx"),
			Kind:     signature.LifecycleTransition,
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("finished"),
		}},
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        "example.transform:return:0:root",
			Objects: []signature.AllocationObjectTemplate{{
				ID:   "example.transform:return:0:root",
				Type: typetable.NewRecord().Build(),
				StaticMembers: []signature.AllocationStaticMemberTemplate{{
					Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "child"}},
					Value:  "example.transform:return:0:root.child",
				}},
				DynamicEntries: []signature.AllocationDynamicEntryTemplate{{
					KeyType: typ.String,
					Value:   "example.transform:return:0:root.entry",
				}},
			}, {
				ID:   "example.transform:return:0:root.child",
				Type: typetable.NewRecord().Build(),
			}, {
				ID:   "example.transform:return:0:root.entry",
				Type: typetable.NewRecord().Build(),
			}},
		}},
	}
	m := New("example/effects")
	m.SetExport(export)
	defineTestTypestateProtocol(t, m, "transaction", []typestate.State{"active", "finished"}, []typestate.State{"finished"}, []typestate.TransitionDecl{{From: "active", To: "finished"}})
	m.DefineFunctionSignature("transform", signature.Function{Type: export, Effect: row, OperationalEffects: operational})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(string(data), `"functionSignatures"`) ||
		!strings.Contains(string(data), `"effect"`) ||
		!strings.Contains(string(data), `"operationalEffects"`) ||
		!strings.Contains(string(data), `"typestateProtocols"`) ||
		!strings.Contains(string(data), `"pathStaticMembers"`) ||
		!strings.Contains(string(data), `"lifecycleEffects"`) ||
		!strings.Contains(string(data), `"returnAllocationTemplates"`) ||
		!strings.Contains(string(data), `"suffix": ".payload"`) {
		t.Fatalf("encoded manifest missing function signature effect data:\n%s", data)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	gotFn, ok := got.Export.(*typ.Function)
	if !ok {
		t.Fatalf("export = %T, want function", got.Export)
	}
	if !typ.TypeEquals(got.Export, export) {
		t.Fatalf("export = %v, want %v", got.Export, export)
	}
	gotSig, ok := got.FunctionSignatures["transform"]
	if !ok {
		t.Fatalf("missing transform function signature")
	}
	if !gotSig.Effect.Equals(row) {
		t.Fatalf("effect = %v, want %v", gotSig.Effect, row)
	}
	if gotSig.OperationalEffects == nil || !gotSig.OperationalEffects.Equals(*operational) {
		t.Fatalf("operational effects = %#v, want %#v", gotSig.OperationalEffects, operational)
	}
	if !typ.TypeEquals(gotSig.Type, gotFn) {
		t.Fatalf("signature type = %v, want %v", gotSig.Type, gotFn)
	}
	if !(signature.Function{Type: export, Effect: row, OperationalEffects: operational}).Equals(gotSig) {
		t.Fatalf("signature = %v, want %v", gotSig, signature.Function{Type: export, Effect: row, OperationalEffects: operational})
	}
	gotProtocol, ok := got.TypestateProtocol("transaction")
	if !ok || !gotProtocol.IsFinal("finished") || !gotProtocol.AllowsTransition("active", "finished") {
		t.Fatalf("typestate protocol = %#v, want transaction FSM", gotProtocol)
	}
}

func TestManifestRoundTripOperationOnlyFunctionSignature(t *testing.T) {
	operational := &signature.OperationalEffects{
		PathInvalidations: []signature.PathInvalidation{{
			Path: pathdom.NewPlaceholder(0),
		}},
	}
	m := New("example/operation-only")
	m.DefineFunctionSignature("install", signature.Function{
		OperationalEffects: operational,
	})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(data), `"type"`) {
		t.Fatalf("operation-only signature serialized a fake type:\n%s", data)
	}
	if !strings.Contains(string(data), `"operationalEffects"`) ||
		!strings.Contains(string(data), `"pathInvalidations"`) {
		t.Fatalf("encoded operation-only signature missing operational effects:\n%s", data)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	gotSig, ok := got.FunctionSignatures["install"]
	if !ok {
		t.Fatalf("missing install signature")
	}
	if gotSig.Type != nil {
		t.Fatalf("signature type = %v, want nil", gotSig.Type)
	}
	if gotSig.OperationalEffects == nil || !gotSig.OperationalEffects.Equals(*operational) {
		t.Fatalf("operational effects = %#v, want %#v", gotSig.OperationalEffects, operational)
	}
}

func TestManifestRejectsInvalidLifecycleOperationalEffects(t *testing.T) {
	tests := []struct {
		name    string
		effects signature.OperationalEffects
		wantErr string
	}{
		{
			name: "missing protocol",
			effects: signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target: pathdom.NewPlaceholder(0),
				Kind:   signature.LifecycleEscape,
			}}},
			wantErr: "missing protocol",
		},
		{
			name: "unsupported kind",
			effects: signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0),
				Kind:     signature.LifecycleNone,
				Protocol: typestate.Protocol("resource"),
			}}},
			wantErr: "unsupported lifecycle kind",
		},
		{
			name: "transition missing target state",
			effects: signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0),
				Kind:     signature.LifecycleTransition,
				Protocol: typestate.Protocol("resource"),
				From:     typestate.State("open"),
			}}},
			wantErr: "transition missing target state",
		},
		{
			name: "transition missing source state",
			effects: signature.OperationalEffects{LifecycleEffects: []signature.LifecycleEffect{{
				Target:   pathdom.NewPlaceholder(0),
				Kind:     signature.LifecycleTransition,
				Protocol: typestate.Protocol("resource"),
				To:       typestate.State("closed"),
			}}},
			wantErr: "transition missing source state",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encodeOperationalEffects(&tt.effects)
			if err == nil {
				t.Fatal("encodeOperationalEffects succeeded, want lifecycle validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}

	decodeTests := []struct {
		name string
		wire lifecycleEffectWire
		want string
	}{
		{"acquire missing protocol", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "acquire", To: "active"}, "missing protocol"},
		{"acquire missing state", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "acquire", Protocol: "resource"}, "acquire missing state"},
		{"transition missing protocol", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "transition", To: "closed"}, "missing protocol"},
		{"transition missing target state", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "transition", Protocol: "resource", From: "open"}, "transition missing target state"},
		{"transition missing source state", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "transition", Protocol: "resource", To: "closed"}, "transition missing source state"},
		{"escape missing protocol", lifecycleEffectWire{Target: &placeholderPathWire{Param: 0}, Kind: "escape"}, "missing protocol"},
	}
	for _, tt := range decodeTests {
		t.Run("decode "+tt.name, func(t *testing.T) {
			_, err := decodeOperationalEffects(&operationalEffectsWire{LifecycleEffects: []lifecycleEffectWire{tt.wire}})
			if err == nil {
				t.Fatal("decodeOperationalEffects succeeded, want lifecycle validation error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestRejectsLifecycleEffectsWithoutDeclaredFSM(t *testing.T) {
	fn := typ.Func().Param("tx", typ.Any).Build()
	m := New("example/lifecycle-missing-fsm")
	m.DefineFunctionSignature("begin", signature.Function{
		Type: fn,
		Effect: effect.Empty.With(lifecycle.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			State:    "active",
			Obligation: typestate.Obligation{
				Final: "finished",
			},
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), `lifecycle protocol "transaction" is not declared as a typestate FSM`) {
		t.Fatalf("Encode error = %v, want undeclared typestate FSM", err)
	}
}

func TestManifestRejectsLifecycleEffectsOutsideDeclaredFSM(t *testing.T) {
	tests := []struct {
		name string
		sig  signature.Function
		want string
	}{
		{
			name: "acquire state not declared",
			sig: signature.Function{Type: typ.Func().Param("tx", typ.Any).Build(), Effect: effect.Empty.With(lifecycle.Acquire{
				Target:   effect.ParamRef{Index: 0},
				Protocol: "transaction",
				State:    "pending",
			})},
			want: `does not declare acquire state "pending"`,
		},
		{
			name: "obligation final not final",
			sig: signature.Function{Type: typ.Func().Param("tx", typ.Any).Build(), Effect: effect.Empty.With(lifecycle.Acquire{
				Target:   effect.ParamRef{Index: 0},
				Protocol: "transaction",
				State:    "active",
				Obligation: typestate.Obligation{
					Final: "active",
				},
			})},
			want: `does not declare obligation final state "active"`,
		},
		{
			name: "transition edge not declared",
			sig: signature.Function{Type: typ.Func().Param("tx", typ.Any).Build(), Effect: effect.Empty.With(lifecycle.Transition{
				Target:   effect.ParamRef{Index: 0},
				Protocol: "transaction",
				From:     "finished",
				To:       "active",
			})},
			want: `does not declare transition "finished" -> "active"`,
		},
		{
			name: "operational transition edge not declared",
			sig: signature.Function{Type: typ.Func().Param("tx", typ.Any).Build(), OperationalEffects: &signature.OperationalEffects{
				LifecycleEffects: []signature.LifecycleEffect{{
					Target:   pathdom.NewPlaceholder(0),
					Kind:     signature.LifecycleTransition,
					Protocol: "transaction",
					From:     "finished",
					To:       "active",
				}},
			}},
			want: `does not declare transition "finished" -> "active"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("example/lifecycle-invalid-fsm")
			defineTestTypestateProtocol(t, m, "transaction", []typestate.State{"active", "finished"}, []typestate.State{"finished"}, []typestate.TransitionDecl{{From: "active", To: "finished"}})
			m.DefineFunctionSignature("f", tt.sig)
			_, err := Encode(m)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestRejectsMalformedTypestateProtocolDeclarations(t *testing.T) {
	m := New("example/bad-fsm")
	err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "cursor",
		States:      []typestate.State{"open"},
		FinalStates: []typestate.State{"closed"},
	})
	if err == nil || !strings.Contains(err.Error(), `final state "closed" is not declared`) {
		t.Fatalf("DefineTypestateProtocol error = %v, want unknown final state", err)
	}
	err = m.DefineTypestateProtocol(typestate.Definition{
		Protocol: "cursor",
		States:   []typestate.State{"open", "open"},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicates state "open"`) {
		t.Fatalf("DefineTypestateProtocol duplicate error = %v, want duplicate state", err)
	}

	_, err = Decode([]byte(`{
  "path": "example/bad-fsm",
  "typestateProtocols": [
    {
      "name": "cursor",
      "states": ["open"],
      "transitions": [{ "from": "open", "to": "closed" }]
    }
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), `transition target "closed" is not declared`) {
		t.Fatalf("Decode error = %v, want unknown transition target", err)
	}
}

func TestManifestEmptyOperationalEffectsAreAbsent(t *testing.T) {
	fn := typ.Func().
		Param("payload", typ.Any).
		Build()
	m := New("example/empty-operational")
	m.DefineFunctionSignature("send", signature.Function{
		Type: fn,
		Effect: effect.Empty.With(ownership.SendParam{
			Param: effect.ParamRef{Index: 0},
		}),
		OperationalEffects: &signature.OperationalEffects{},
	})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(data), `"operationalEffects"`) {
		t.Fatalf("empty operational effects were serialized as authority:\n%s", data)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	gotSig, ok := got.FunctionSignatures["send"]
	if !ok {
		t.Fatalf("missing send signature")
	}
	if gotSig.OperationalEffects != nil {
		t.Fatalf("decoded operational effects = %#v, want nil for empty DTO", gotSig.OperationalEffects)
	}
	if !gotSig.Effect.Equals(m.FunctionSignatures["send"].Effect) {
		t.Fatalf("effect row = %v, want %v", gotSig.Effect, m.FunctionSignatures["send"].Effect)
	}

	encodedType, err := encodeType(fn)
	if err != nil {
		t.Fatalf("encodeType: %v", err)
	}
	decoded, err := decodeFunctionSignature(functionSignatureWire{
		Name:               "send",
		Type:               encodedType,
		OperationalEffects: &operationalEffectsWire{},
	})
	if err != nil {
		t.Fatalf("decodeFunctionSignature: %v", err)
	}
	if decoded.OperationalEffects != nil {
		t.Fatalf("explicit empty operational wire decoded as %#v, want nil", decoded.OperationalEffects)
	}
}

func TestManifestOperationalPathStaticMemberRequiresType(t *testing.T) {
	fn := typ.Func().
		Param("payload", typ.Any).
		Build()
	encodedType, err := encodeType(fn)
	if err != nil {
		t.Fatalf("encodeType: %v", err)
	}
	_, err = decodeFunctionSignature(functionSignatureWire{
		Name: "static-member",
		Type: encodedType,
		OperationalEffects: &operationalEffectsWire{
			PathStaticMembers: []pathStaticMemberWire{{
				Path: &placeholderPathWire{Param: 0, Suffix: ".field"},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "path static member type: missing") {
		t.Fatalf("decodeFunctionSignature error = %v, want missing static-member type", err)
	}
}

func TestManifestOperationalAllocationTemplateRequiresRootObject(t *testing.T) {
	fn := typ.Func().Returns(typ.Any).Build()
	_, err := decodeFunctionSignature(functionSignatureWire{
		Name: "allocation-template",
		Type: testEncodedFunctionType(t, fn),
		OperationalEffects: &operationalEffectsWire{
			ReturnAllocationTemplates: []returnAllocationTemplateWire{{
				ReturnIndex: 0,
				Root:        "missing",
				Objects: []allocationObjectWire{{
					ID: "other",
				}},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `root "missing" has no object template`) {
		t.Fatalf("decodeFunctionSignature error = %v, want missing allocation root object", err)
	}
}

func TestManifestOperationalAllocationTemplateRejectsInvalidGraph(t *testing.T) {
	fn := typ.Func().Returns(typ.Any).Build()
	tests := []struct {
		name string
		wire returnAllocationTemplateWire
		want string
	}{
		{
			name: "return index out of bounds",
			wire: returnAllocationTemplateWire{
				ReturnIndex: 1,
				Root:        "root",
				Objects:     []allocationObjectWire{{ID: "root"}},
			},
			want: "return allocation template index 1 out of bounds for 1 returns",
		},
		{
			name: "dangling static member value",
			wire: returnAllocationTemplateWire{
				ReturnIndex: 0,
				Root:        "root",
				Objects: []allocationObjectWire{{
					ID: "root",
					StaticMembers: []allocationStaticMemberWire{{
						Suffix: ".child",
						Value:  "missing-child",
					}},
				}},
			},
			want: `static member .child references missing object "missing-child"`,
		},
		{
			name: "dangling dynamic key",
			wire: returnAllocationTemplateWire{
				ReturnIndex: 0,
				Root:        "root",
				Objects: []allocationObjectWire{{
					ID: "root",
					DynamicEntries: []allocationDynamicEntryWire{{
						Key:   "missing-key",
						Value: "value",
					}},
				}, {
					ID: "value",
				}},
			},
			want: `dynamic entry references missing key object "missing-key"`,
		},
		{
			name: "dangling dynamic value",
			wire: returnAllocationTemplateWire{
				ReturnIndex: 0,
				Root:        "root",
				Objects: []allocationObjectWire{{
					ID: "root",
					DynamicEntries: []allocationDynamicEntryWire{{
						Value: "missing-value",
					}},
				}},
			},
			want: `dynamic entry references missing value object "missing-value"`,
		},
		{
			name: "empty dynamic entry",
			wire: returnAllocationTemplateWire{
				ReturnIndex: 0,
				Root:        "root",
				Objects: []allocationObjectWire{{
					ID:             "root",
					DynamicEntries: []allocationDynamicEntryWire{{}},
				}},
			},
			want: `dynamic entry missing key, key type, or value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeFunctionSignature(functionSignatureWire{
				Name: "allocation-template",
				Type: testEncodedFunctionType(t, fn),
				OperationalEffects: &operationalEffectsWire{
					ReturnAllocationTemplates: []returnAllocationTemplateWire{tt.wire},
				},
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeFunctionSignature error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestOperationalAllocationTemplateRejectsInvalidDirectGraph(t *testing.T) {
	fn := typ.Func().Returns(typ.Any).Build()
	tests := []struct {
		name     string
		template signature.ReturnAllocationTemplate
		want     string
	}{
		{
			name: "root object missing",
			template: signature.ReturnAllocationTemplate{
				ReturnIndex: 0,
				Root:        "missing",
				Objects:     []signature.AllocationObjectTemplate{{ID: "other"}},
			},
			want: `root "missing" has no object template`,
		},
		{
			name: "empty dynamic entry",
			template: signature.ReturnAllocationTemplate{
				ReturnIndex: 0,
				Root:        "root",
				Objects: []signature.AllocationObjectTemplate{{
					ID:             "root",
					DynamicEntries: []signature.AllocationDynamicEntryTemplate{{}},
				}},
			},
			want: `dynamic entry missing key, key type, or value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New("allocation-template")
			m.DefineFunctionSignature("make", signature.Function{
				Type: fn,
				OperationalEffects: &signature.OperationalEffects{
					ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{tt.template},
				},
			})

			if err := m.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate error = %v, want %q", err, tt.want)
			}
			if _, err := Encode(m); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Encode error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestOperationalAllocationTemplateRejectsDuplicateReturnIndex(t *testing.T) {
	fn := typ.Func().Returns(typ.Any).Build()
	_, err := decodeFunctionSignature(functionSignatureWire{
		Name: "allocation-template",
		Type: testEncodedFunctionType(t, fn),
		OperationalEffects: &operationalEffectsWire{
			ReturnAllocationTemplates: []returnAllocationTemplateWire{{
				ReturnIndex: 0,
				Root:        "left",
				Objects:     []allocationObjectWire{{ID: "left"}},
			}, {
				ReturnIndex: 0,
				Root:        "right",
				Objects:     []allocationObjectWire{{ID: "right"}},
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate return allocation template for return index 0") {
		t.Fatalf("decodeFunctionSignature error = %v, want duplicate return allocation template", err)
	}
}

func testEncodedFunctionType(t *testing.T, fn *typ.Function) *typeWire {
	t.Helper()
	encodedType, err := encodeType(fn)
	if err != nil {
		t.Fatalf("encodeType: %v", err)
	}
	return encodedType
}

func defineTestTypestateProtocol(t *testing.T, m *Manifest, protocol typestate.Protocol, states, finals []typestate.State, transitions []typestate.TransitionDecl) {
	t.Helper()
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    protocol,
		States:      states,
		FinalStates: finals,
		Transitions: transitions,
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol(%s): %v", protocol, err)
	}
}

func defineOperationalOrderTypestateProtocols(t *testing.T, m *Manifest) {
	t.Helper()
	defineTestTypestateProtocol(t, m, "transaction", []typestate.State{"active", "finished"}, []typestate.State{"finished"}, []typestate.TransitionDecl{{From: "active", To: "finished"}})
	defineTestTypestateProtocol(t, m, "socket", []typestate.State{"open", "closed"}, []typestate.State{"closed"}, []typestate.TransitionDecl{{From: "open", To: "closed"}})
}

func TestManifestOperationalEffectsEncodeDeterministically(t *testing.T) {
	fn := typ.Func().
		Param("left", typ.Any).
		Param("right", typ.Any).
		Returns(typeexpr.Optional(typ.Number), typeexpr.Optional(typ.String)).
		Build()
	left := New("example/deterministic-operational")
	defineOperationalOrderTypestateProtocols(t, left)
	left.DefineFunctionSignature("f", signature.Function{
		Type:               fn,
		OperationalEffects: operationalEffectsOrderA(),
	})
	right := New("example/deterministic-operational")
	defineOperationalOrderTypestateProtocols(t, right)
	right.DefineFunctionSignature("f", signature.Function{
		Type:               fn,
		OperationalEffects: operationalEffectsOrderB(),
	})

	leftData, err := Encode(left)
	if err != nil {
		t.Fatalf("Encode(left): %v", err)
	}
	rightData, err := Encode(right)
	if err != nil {
		t.Fatalf("Encode(right): %v", err)
	}
	if !bytes.Equal(leftData, rightData) {
		t.Fatalf("operational effects encoding is not stable:\nleft:\n%s\nright:\n%s", leftData, rightData)
	}
}

func TestManifestEffectLabelRoundTripPreservesRowsAndSelectors(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	cases := []struct {
		name  string
		label effect.Label
	}{
		{"dispatch module load", dispatch.ModuleLoad{}},
		{"iteration iterator", iteration.Iterator{Source: p0, Kind: iteration.IterateIndexed}},
		{"mutation mutate", mutation.Mutate{Target: p0, Transform: mutation.ElementUnion{Source: p1}, LengthDelta: expr.Add(expr.PL(0), expr.C(1))}},
		{"mutation length change", mutation.LengthChange{Target: p1, Delta: -2}},
		{"mutation table mutator", mutation.TableMutator{Target: p0, Value: p1}},
		{"ownership borrow", ownership.Borrow{Param: p0}},
		{"ownership retain", ownership.Retain{Param: p0}},
		{"ownership store", ownership.Store{Param: p0, Into: p1}},
		{"ownership borrow all", ownership.BorrowAll{}},
		{"ownership send", ownership.Send{FromParam: 1}},
		{"ownership send param", ownership.SendParam{Param: p2}},
		{"ownership export", ownership.Export{Param: p0}},
		{"ownership opaque", ownership.Opaque{Param: p1}},
		{"ownership freeze", ownership.Freeze{Param: p2}},
		{"postcondition normal return present", postcondition.NormalReturnRefinement{Target: p0, Refinement: postcondition.Present{}}},
		{"postcondition normal return absent", postcondition.NormalReturnRefinement{Target: p1, Refinement: postcondition.Absent{}}},
		{"returns return", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"returns error return", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if got.Hash() != row.Hash() {
				t.Fatalf("roundtrip hash = %d, want %d", got.Hash(), row.Hash())
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func operationalEffectsOrderA() *signature.OperationalEffects {
	return &signature.OperationalEffects{
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{
			{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			{TriggerIndex: 0, TriggerPresence: presence.Absent(), TargetIndex: 1, TargetPresence: presence.Present()},
		},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{
			{Path: pathdom.NewPlaceholder(1).Field("ready"), Presence: presence.Present()},
			{Path: pathdom.NewPlaceholder(0).Field("missing"), Presence: presence.Absent()},
		},
		PathStaticMembers: []signature.PathStaticMemberFact{
			{Path: pathdom.NewPlaceholder(1).Field("kind"), Type: typ.String},
			{Path: pathdom.NewPlaceholder(0).Field("kind"), Type: typeexpr.Optional(typ.Number)},
		},
		PathInvalidations: []signature.PathInvalidation{
			{Path: pathdom.NewPlaceholder(1).Field("items")},
			{Path: pathdom.NewPlaceholder(0).Field("items")},
		},
		FrozenTables: []signature.FrozenTable{
			{Target: pathdom.NewPlaceholder(1).Field("sealed")},
			{Target: pathdom.NewPlaceholder(0).Field("sealed")},
		},
		EscapeEvents: []signature.EscapeEvent{
			{Target: pathdom.NewPlaceholder(1).Field("payload"), Kind: signature.EscapeSend, Recursive: true},
			{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: signature.EscapeBorrow},
		},
		StoreRelations: []signature.StoreRelation{
			{Source: pathdom.NewPlaceholder(1).Field("payload"), Into: pathdom.NewPlaceholder(0).Field("bucket")},
			{Source: pathdom.NewPlaceholder(0).Field("payload"), Into: pathdom.NewPlaceholder(1).Field("bucket")},
		},
		LifecycleEffects: []signature.LifecycleEffect{
			{
				Target:   pathdom.NewPlaceholder(1).Field("resource"),
				Kind:     signature.LifecycleTransition,
				Protocol: typestate.Protocol("socket"),
				From:     typestate.State("open"),
				To:       typestate.State("closed"),
			},
			{
				Target:   pathdom.NewPlaceholder(0).Field("tx"),
				Kind:     signature.LifecycleAcquire,
				Protocol: typestate.Protocol("transaction"),
				To:       typestate.State("active"),
				Obligation: typestate.Obligation{
					Final: typestate.State("finished"),
				},
			},
		},
	}
}

func operationalEffectsOrderB() *signature.OperationalEffects {
	return &signature.OperationalEffects{
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{
			{TriggerIndex: 0, TriggerPresence: presence.Absent(), TargetIndex: 1, TargetPresence: presence.Present()},
			{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
		},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{
			{Path: pathdom.NewPlaceholder(0).Field("missing"), Presence: presence.Absent()},
			{Path: pathdom.NewPlaceholder(1).Field("ready"), Presence: presence.Present()},
		},
		PathStaticMembers: []signature.PathStaticMemberFact{
			{Path: pathdom.NewPlaceholder(0).Field("kind"), Type: typeexpr.Optional(typ.Number)},
			{Path: pathdom.NewPlaceholder(1).Field("kind"), Type: typ.String},
		},
		PathInvalidations: []signature.PathInvalidation{
			{Path: pathdom.NewPlaceholder(0).Field("items")},
			{Path: pathdom.NewPlaceholder(1).Field("items")},
		},
		FrozenTables: []signature.FrozenTable{
			{Target: pathdom.NewPlaceholder(0).Field("sealed")},
			{Target: pathdom.NewPlaceholder(1).Field("sealed")},
		},
		EscapeEvents: []signature.EscapeEvent{
			{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: signature.EscapeBorrow},
			{Target: pathdom.NewPlaceholder(1).Field("payload"), Kind: signature.EscapeSend, Recursive: true},
		},
		StoreRelations: []signature.StoreRelation{
			{Source: pathdom.NewPlaceholder(0).Field("payload"), Into: pathdom.NewPlaceholder(1).Field("bucket")},
			{Source: pathdom.NewPlaceholder(1).Field("payload"), Into: pathdom.NewPlaceholder(0).Field("bucket")},
		},
		LifecycleEffects: []signature.LifecycleEffect{
			{
				Target:   pathdom.NewPlaceholder(0).Field("tx"),
				Kind:     signature.LifecycleAcquire,
				Protocol: typestate.Protocol("transaction"),
				To:       typestate.State("active"),
				Obligation: typestate.Obligation{
					Final: typestate.State("finished"),
				},
			},
			{
				Target:   pathdom.NewPlaceholder(1).Field("resource"),
				Kind:     signature.LifecycleTransition,
				Protocol: typestate.Protocol("socket"),
				From:     typestate.State("open"),
				To:       typestate.State("closed"),
			},
		},
	}
}

func rowHasLabel(row effect.Row, want effect.Label) bool {
	want = effect.NormalizeLabel(want)
	return row.Has(func(got effect.Label) bool {
		return got != nil && got.Equals(want)
	})
}

func TestManifestEffectLabelRoundTripCoversNestedKinds(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	rows := []effect.Row{
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.Unchanged{}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ContainerElementUnion{Container: p1, Value: p2}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ToArray{Element: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
				projection.GenericArg(0),
				projection.InstantiateGeneric(typ.String),
			}},
		}}),
	}

	for _, row := range rows {
		t.Run(row.String(), func(t *testing.T) {
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
		})
	}
}

func TestManifestEffectLabelRoundTripCoversActiveReturnMatrix(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	tests := []struct {
		name   string
		status string
		label  effect.Label
	}{
		{"actively lowered", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"actively lowered optional", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}},
		{"actively lowered callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p1}}},
		{"actively lowered array callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p1}}},
		{"actively lowered same as", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p0}}},
		{"actively lowered projection", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
			}},
		}}},
		{"error return", "actively lowered by effectlowering", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
		{"lifecycle acquire", "actively lowered by effectlowering", lifecycle.Acquire{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}},
		{"lifecycle transition", "actively lowered by effectlowering", lifecycle.Transition{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
			From:     typestate.State("active"),
			To:       typestate.State("finished"),
		}},
		{"lifecycle escape", "actively lowered by effectlowering", lifecycle.Escape{
			Target:   p0,
			Protocol: typestate.Protocol("transaction"),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.status+" / "+tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func TestManifestRejectsInactiveEffectLabels(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	tests := []struct {
		name  string
		label effect.Label
	}{
		{"control throw", control.Throw{}},
		{"control io", control.IO{}},
		{"dispatch type predicate", dispatch.TypePredicate{}},
		{"dispatch variadic transform", dispatch.VariadicTransform{}},
		{"return deep element", returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{Source: p0}}},
		{"return string unpack", returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{Format: p2}}},
		{"return select case", returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{Source: p0}}},
		{"return select result", returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{Cases: p0, Default: p1}}},
		{"return length", returns.ReturnLength{ReturnIndex: 0, Length: expr.MinExpr(expr.PL(0), expr.C(3))}},
		{"correlated return", returns.CorrelatedReturn{Indices: []int{0, 2}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := effect.Empty.With(tt.label)
			if _, err := encodeEffectRow(row); err == nil {
				t.Fatalf("encodeEffectRow(%v) succeeded, want inactive-label rejection", row)
			} else if !strings.Contains(err.Error(), "inactive effect label") {
				t.Fatalf("encodeEffectRow error = %v, want inactive-label rejection", err)
			}
		})
	}
}

func TestManifestRejectsInactiveDecodedEffectLabels(t *testing.T) {
	tests := []struct {
		name string
		wire effectLabelWire
	}{
		{"control throw", effectLabelWire{Kind: "control.throw"}},
		{"control io", effectLabelWire{Kind: "control.io"}},
		{"dispatch type predicate", effectLabelWire{Kind: "dispatch.typePredicate"}},
		{"dispatch variadic transform", effectLabelWire{Kind: "dispatch.variadicTransform"}},
		{"return length", effectLabelWire{Kind: "returns.returnLength", Length: encodeExprForTest(t, expr.C(1))}},
		{"correlated return", effectLabelWire{Kind: "returns.correlatedReturn", Indices: []int{0, 1}}},
		{"return deep element", effectLabelWire{Kind: "returns.return", ReturnType: &effectReturnWire{Kind: "returns.deepElementOf"}}},
		{"return string unpack", effectLabelWire{Kind: "returns.return", ReturnType: &effectReturnWire{Kind: "returns.stringUnpackValue"}}},
		{"return select case", effectLabelWire{Kind: "returns.return", ReturnType: &effectReturnWire{Kind: "returns.selectCaseOfParam"}}},
		{"return select result", effectLabelWire{Kind: "returns.return", ReturnType: &effectReturnWire{Kind: "returns.selectResultOfCases"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil {
				t.Fatal("decodeEffectRow succeeded, want inactive-label rejection")
			}
			if !strings.Contains(err.Error(), "inactive effect label") {
				t.Fatalf("decodeEffectRow error = %v, want inactive-label rejection", err)
			}
		})
	}
}

func TestManifestRejectsMalformedLifecycleEffectLabels(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	encodeTests := []struct {
		name  string
		label effect.Label
		want  string
	}{
		{"acquire missing protocol", lifecycle.Acquire{Target: p0, State: typestate.State("active")}, "missing protocol"},
		{"acquire missing state", lifecycle.Acquire{Target: p0, Protocol: typestate.Protocol("transaction")}, "missing state"},
		{"transition missing protocol", lifecycle.Transition{Target: p0, To: typestate.State("finished")}, "missing protocol"},
		{"transition missing target", lifecycle.Transition{Target: p0, Protocol: typestate.Protocol("transaction")}, "missing target state"},
		{"transition missing source", lifecycle.Transition{Target: p0, Protocol: typestate.Protocol("transaction"), To: typestate.State("finished")}, "missing source state"},
		{"escape missing protocol", lifecycle.Escape{Target: p0}, "missing protocol"},
	}
	for _, tt := range encodeTests {
		t.Run("encode "+tt.name, func(t *testing.T) {
			_, err := encodeEffectRow(effect.Empty.With(tt.label))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("encodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}

	decodeTests := []struct {
		name string
		wire effectLabelWire
		want string
	}{
		{"acquire missing protocol", effectLabelWire{Kind: "lifecycle.acquire", Target: encodeParamRef(p0), To: "active"}, "missing protocol"},
		{"acquire missing state", effectLabelWire{Kind: "lifecycle.acquire", Target: encodeParamRef(p0), Protocol: "transaction"}, "missing state"},
		{"transition missing protocol", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), To: "finished"}, "missing protocol"},
		{"transition missing target", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), Protocol: "transaction"}, "missing target state"},
		{"transition missing source", effectLabelWire{Kind: "lifecycle.transition", Target: encodeParamRef(p0), Protocol: "transaction", To: "finished"}, "missing source state"},
		{"escape missing protocol", effectLabelWire{Kind: "lifecycle.escape", Target: encodeParamRef(p0)}, "missing protocol"},
	}
	for _, tt := range decodeTests {
		t.Run("decode "+tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestRejectsEffectLabelsMissingParamRefs(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	tests := []struct {
		name string
		wire effectLabelWire
		want string
	}{
		{
			name: "iterator source",
			wire: effectLabelWire{Kind: "iteration.iterator", IteratorKind: "indexed"},
			want: "iterator source missing param ref",
		},
		{
			name: "mutation target",
			wire: effectLabelWire{Kind: "mutation.lengthChange"},
			want: "length change target missing param ref",
		},
		{
			name: "table mutator value",
			wire: effectLabelWire{Kind: "mutation.tableMutator", Target: encodeParamRef(p0)},
			want: "table mutator value missing param ref",
		},
		{
			name: "mutation transform source",
			wire: effectLabelWire{
				Kind:      "mutation.mutate",
				Target:    encodeParamRef(p0),
				Transform: &effectTransformWire{Kind: "mutation.elementUnion"},
				Length:    encodeExprForTest(t, expr.C(0)),
			},
			want: "mutation.elementUnion source missing param ref",
		},
		{
			name: "lifecycle target",
			wire: effectLabelWire{Kind: "lifecycle.acquire", Protocol: "transaction", To: "active"},
			want: "lifecycle acquire target missing param ref",
		},
		{
			name: "ownership store target",
			wire: effectLabelWire{Kind: "ownership.store", Param: encodeParamRef(p0)},
			want: "store target missing param ref",
		},
		{
			name: "ownership send fromParam",
			wire: effectLabelWire{Kind: "ownership.send"},
			want: "send fromParam missing",
		},
		{
			name: "return transform source",
			wire: effectLabelWire{
				Kind:       "returns.return",
				ReturnType: &effectReturnWire{Kind: "returns.elementOf"},
			},
			want: "returns.elementOf source missing param ref",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeEffectRow(&effectRowWire{Labels: []effectLabelWire{tt.wire}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeEffectRow error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManifestEffectLabelSendEncodesZeroFromParamExplicitly(t *testing.T) {
	wire, err := encodeEffectRow(effect.Empty.With(ownership.Send{FromParam: 0}))
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	if wire == nil || len(wire.Labels) != 1 || wire.Labels[0].FromParam == nil || *wire.Labels[0].FromParam != 0 {
		t.Fatalf("send label wire = %#v, want explicit fromParam 0", wire)
	}
}

func TestManifestEffectPointerLabelsNormalizeToValues(t *testing.T) {
	row := effect.Row{Labels: []effect.Label{
		&iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed},
		&mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: &mutation.ToArray{Element: effect.ParamRef{Index: 1}}},
		&ownership.Borrow{Param: effect.ParamRef{Index: 2}},
		&returns.Return{ReturnIndex: 0, Transform: &returns.ElementOf{Source: effect.ParamRef{Index: 0}}},
	}}

	got := mustRoundTripEffectRow(t, row)
	if !got.Equals(row) {
		t.Fatalf("roundtrip pointer row = %v, want %v", got, row)
	}
	if got.Hash() != row.Hash() {
		t.Fatalf("roundtrip pointer hash = %d, want %d", got.Hash(), row.Hash())
	}
	for _, want := range row.Labels {
		if !rowHasLabel(got, want) {
			t.Fatalf("roundtrip pointer row missing %T in %v", want, got)
		}
	}
	for _, label := range got.Labels {
		if effect.NormalizeLabel(label) != label {
			t.Fatalf("decoded label %T was not value-owned", label)
		}
	}
}

func TestManifestExprPointerRoundTrip(t *testing.T) {
	original := &expr.BinOp{
		Op:    expr.OpAdd,
		Left:  &expr.ParamLen{Index: 0},
		Right: &expr.Const{Value: 2},
	}

	wire, err := encodeExpr(original)
	if err != nil {
		t.Fatalf("encodeExpr(pointer): %v", err)
	}
	got, err := decodeExpr(wire)
	if err != nil {
		t.Fatalf("decodeExpr(pointer roundtrip): %v", err)
	}
	if got.String() != original.String() {
		t.Fatalf("roundtrip expr = %s, want %s", got, original)
	}
}

func TestManifestExprRejectsTypedNilPointer(t *testing.T) {
	var typedNil *expr.Const
	if _, err := encodeExpr(typedNil); err == nil {
		t.Fatal("encodeExpr(typed nil) succeeded, want error")
	} else if !strings.Contains(err.Error(), "nil constraint expr") {
		t.Fatalf("encodeExpr(typed nil) error = %v", err)
	}
}

func TestManifestExprRejectsMissingCompoundOperands(t *testing.T) {
	one := encodeExprForTest(t, expr.C(1))
	tests := []struct {
		name string
		wire *exprWire
		want string
	}{
		{
			name: "binop missing left",
			wire: &exprWire{Kind: "binop", Op: "+", Right: one},
			want: "binop left",
		},
		{
			name: "binop missing right",
			wire: &exprWire{Kind: "binop", Op: "+", Left: one},
			want: "binop right",
		},
		{
			name: "min missing left",
			wire: &exprWire{Kind: "min", Right: one},
			want: "min left",
		},
		{
			name: "min missing right",
			wire: &exprWire{Kind: "min", Left: one},
			want: "min right",
		},
		{
			name: "max missing left",
			wire: &exprWire{Kind: "max", Right: one},
			want: "max left",
		},
		{
			name: "max missing right",
			wire: &exprWire{Kind: "max", Left: one},
			want: "max right",
		},
	}

	if got, err := decodeExpr(nil); err != nil || got != nil {
		t.Fatalf("decodeExpr(nil) = %#v/%v, want nil/nil for optional top-level field", got, err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeExpr(tt.wire); err == nil {
				t.Fatal("decodeExpr succeeded, want missing operand error")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decodeExpr error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestManifestExprRejectsInvalidEncodeOp(t *testing.T) {
	_, err := encodeExpr(expr.BinOp{Op: expr.Op(99), Left: expr.C(1), Right: expr.C(2)})
	if err == nil {
		t.Fatal("encodeExpr(invalid op) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "unsupported expr op") {
		t.Fatalf("encodeExpr(invalid op) error = %v", err)
	}
}

func encodeExprForTest(t *testing.T, e expr.Expr) *exprWire {
	t.Helper()
	wire, err := encodeExpr(e)
	if err != nil {
		t.Fatalf("encodeExpr: %v", err)
	}
	return wire
}

func mustRoundTripEffectRow(t *testing.T, row effect.Row) effect.Row {
	t.Helper()
	wire, err := encodeEffectRow(row)
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	got, err := decodeEffectRow(wire)
	if err != nil {
		t.Fatalf("decodeEffectRow: %v", err)
	}
	return got
}

func TestManifestEncodeUnknownIteratorKindErrors(t *testing.T) {
	m := New("example/bad-iterator")
	m.DefineFunctionSignature("iter", signature.Function{
		Type: typ.Func().
			Param("input", typ.NewArray(typ.String)).
			Build(),
		Effect: effect.Empty.With(iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IteratorKind(99),
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), "unknown iterator kind 99") {
		t.Fatalf("Encode error = %v, want unknown iterator kind", err)
	}
}

func TestManifestDecodeIteratorRequiresExplicitKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{Kind: "iteration.iterator"})
	if err == nil || !strings.Contains(err.Error(), `unknown iterator kind ""`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown iterator kind", err)
	}
}

func TestManifestDecodePostconditionRefinementRequiresKnownKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:       postcondition.NormalReturnRefinementKind,
		Target:     &paramRefWire{Index: 0},
		Refinement: &effectRefinementWire{Kind: "future"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown effect refinement kind "future"`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown effect refinement kind", err)
	}
}

func TestManifestDecodePostconditionRefinementRequiresRefinement(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:   postcondition.NormalReturnRefinementKind,
		Target: &paramRefWire{Index: 0},
	})
	if err == nil || !strings.Contains(err.Error(), "missing effect refinement") {
		t.Fatalf("decodeEffectLabel error = %v, want missing effect refinement", err)
	}
}

func TestManifestEncodePostconditionRefinementRequiresRefinement(t *testing.T) {
	m := New("example/bad-postcondition")
	m.DefineFunctionSignature("assertLike", signature.Function{
		Type: typ.Func().
			Param("input", typ.Any).
			Build(),
		Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
			Target: effect.ParamRef{Index: 0},
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), "missing effect refinement") {
		t.Fatalf("Encode error = %v, want missing effect refinement", err)
	}
}

func TestManifestNilHandling(t *testing.T) {
	if _, err := Encode(nil); err == nil {
		t.Fatalf("Encode(nil) succeeded")
	}
	if _, err := Decode(nil); err == nil {
		t.Fatalf("Decode(nil) succeeded")
	}
	if _, err := Decode([]byte("   \n\t")); err == nil {
		t.Fatalf("Decode(blank) succeeded")
	}
}

func TestManifestEncodeOrdersNamedTypesDeterministically(t *testing.T) {
	left := New("example/module")
	left.DefineType("Zed", typ.String)
	left.DefineType("Alpha", typ.Number)
	left.DefineType("Middle", typ.Boolean)

	right := New("example/module")
	right.DefineType("Middle", typ.Boolean)
	right.DefineType("Alpha", typ.Number)
	right.DefineType("Zed", typ.String)

	leftData, err := Encode(left)
	if err != nil {
		t.Fatalf("Encode(left): %v", err)
	}
	rightData, err := Encode(right)
	if err != nil {
		t.Fatalf("Encode(right): %v", err)
	}
	if !bytes.Equal(leftData, rightData) {
		t.Fatalf("encoding is not stable:\nleft:\n%s\nright:\n%s", leftData, rightData)
	}

	text := string(leftData)
	alpha := strings.Index(text, `"name": "Alpha"`)
	middle := strings.Index(text, `"name": "Middle"`)
	zed := strings.Index(text, `"name": "Zed"`)
	if alpha < 0 || middle < 0 || zed < 0 || !(alpha < middle && middle < zed) {
		t.Fatalf("named types are not sorted:\n%s", text)
	}
}
