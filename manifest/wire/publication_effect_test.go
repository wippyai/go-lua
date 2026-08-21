package wire

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

func TestPublicationEffectManifestRoundTripAndCloneIsolation(t *testing.T) {
	m := New("publication/module")
	fn := typ.Func().Param("value", typ.Any).Param("context", typ.Any).Returns(typ.Any).Build()
	m.DefineFunctionSignature("sink", signature.Function{Type: fn})
	m.DefineFunctionSignature("effect-target", signature.Function{Type: fn})
	want := PublicationEffectSpec{
		Kind:        PublicationEffectSendTransfer,
		Subject:     InputSource{Kind: InputSourceValue, Ordinal: 0},
		Destination: PublicationDestinationValueFormal,
		Context:     1,
		Escape:      PublicationEscapeSendTransfer,
		Mutability:  PublicationMutabilityCopyOnWrite,
		Lifetime:    PublicationLifetimePreserve,
	}
	operation := Operation{Effects: RowSpec{
		Occurrences: []EffectSpec{{
			Target:      "effect-target",
			ValueArgs:   []ValueFormal{0, 1},
			Publication: &want,
		}},
		Tail: RowClosed,
	}}
	m.DefineFunctionOperation("sink", operation)

	// The manifest owns the nested row. Mutating the caller's descriptor after
	// registration must not alter the provider-owned declaration.
	want.Mutability = PublicationMutabilityPreserve
	stored := m.FunctionOperations["sink"]
	if stored.Effects.Occurrences[0].Publication == nil ||
		stored.Effects.Occurrences[0].Publication.Mutability != PublicationMutabilityCopyOnWrite {
		t.Fatalf("stored publication = %#v, want an ownership-isolated descriptor", stored.Effects.Occurrences[0].Publication)
	}

	encoded, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"schemaRevision"`)) || !bytes.Contains(encoded, []byte(`"wireRevision"`)) {
		t.Fatalf("publication manifest omitted wire revision marker:\n%s", encoded)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	got := decoded.FunctionOperations["sink"]
	if len(got.Effects.Occurrences) != 1 || got.Effects.Occurrences[0].Target != "effect-target" {
		t.Fatalf("decoded effect rows = %#v", got.Effects.Occurrences)
	}
	if got.Effects.Occurrences[0].Publication == nil || *got.Effects.Occurrences[0].Publication != (PublicationEffectSpec{
		Kind:        PublicationEffectSendTransfer,
		Subject:     InputSource{Kind: InputSourceValue, Ordinal: 0},
		Destination: PublicationDestinationValueFormal,
		Context:     1,
		Escape:      PublicationEscapeSendTransfer,
		Mutability:  PublicationMutabilityCopyOnWrite,
		Lifetime:    PublicationLifetimePreserve,
	}) {
		t.Fatalf("decoded publication = %#v", got.Effects.Occurrences[0].Publication)
	}
	reencoded, err := Encode(decoded)
	if err != nil {
		t.Fatalf("re-Encode: %v", err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatalf("publication manifest changed across round trip:\n%s\n%s", encoded, reencoded)
	}
}

func TestPublicationEffectManifestRejectsUnknownNestedWireField(t *testing.T) {
	_, err := Decode([]byte(`{
		"path":"publication/invalid",
		"functionOperations":[{
			"name":"sink",
			"operation":{"effects":{"occurrences":[{"target":"effect-target","publication":{"unknown":1}}],"tail":1}}
		}]
	}`))
	if err == nil || !strings.Contains(err.Error(), `unknown field "unknown"`) {
		t.Fatalf("Decode error = %v, want strict nested publication-field rejection", err)
	}
}

func TestPublicationEffectManifestRequiresSupportedOperationWireRevision(t *testing.T) {
	withoutRevision := []byte(`{
		"path":"publication/no-revision",
		"functionOperations":[{"name":"sink","operation":{"wireRevision":2,"effects":{"occurrences":[{"target":"effect-target"}],"tail":1}}}]
	}`)
	if _, err := Decode(withoutRevision); err == nil || !strings.Contains(err.Error(), "require schema revision") {
		t.Fatalf("Decode without revision error = %v, want revision gate", err)
	}
	unsupported := []byte(`{
		"path":"publication/unsupported-revision",
		"schemaRevision":3,
		"functionOperations":[{"name":"sink","operation":{"wireRevision":2,"effects":{"occurrences":[{"target":"effect-target"}],"tail":1}}}]
	}`)
	if _, err := Decode(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported schema revision") {
		t.Fatalf("Decode unsupported revision error = %v, want unsupported revision gate", err)
	}
	operationUnsupported := []byte(`{
		"path":"publication/unsupported-operation-revision",
		"schemaRevision":2,
		"functionOperations":[{"name":"sink","operation":{"wireRevision":3,"effects":{"occurrences":[{"target":"effect-target"}],"tail":1}}}]
	}`)
	if _, err := Decode(operationUnsupported); err == nil || !strings.Contains(err.Error(), "unsupported operation wire revision") {
		t.Fatalf("Decode unsupported operation revision error = %v, want operation revision gate", err)
	}
}

func TestPublicationEffectManifestPreservesCallbackRows(t *testing.T) {
	row := RowSpec{Occurrences: []EffectSpec{{Target: "effect-target", Publication: &PublicationEffectSpec{
		Kind: PublicationEffectReturnEscape, Subject: InputSource{Kind: InputSourceValue, Ordinal: 0}, Destination: PublicationDestinationNone,
		Escape: PublicationEscapeReturn, Mutability: PublicationMutabilityPreserve, Lifetime: PublicationLifetimePreserve,
	}}}, Tail: RowClosed}
	operation := Operation{Callbacks: []Callback{{Effects: row}}}
	cloned := CloneOperation(operation)
	if len(cloned.Callbacks) != 1 || len(cloned.Callbacks[0].Effects.Occurrences) != 1 {
		t.Fatalf("cloned callback rows = %#v", cloned.Callbacks)
	}
	operation.Callbacks[0].Effects.Occurrences[0].Publication.Subject.Ordinal = 9
	if cloned.Callbacks[0].Effects.Occurrences[0].Publication.Subject.Ordinal != 0 {
		t.Fatal("CloneOperation callback publication aliases source")
	}
}

func TestPublicationEffectManifestRejectsDuplicateOperationNames(t *testing.T) {
	data := []byte(`{
		"path":"publication/duplicate-operation",
		"functionOperations":[
			{"name":"sink","operation":{}},
			{"name":"sink","operation":{"selfEffect":true}}
		]
	}`)
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), `duplicate function operation "sink"`) {
		t.Fatalf("Decode duplicate operation error = %v, want duplicate-name rejection", err)
	}
}

func TestPublicationEffectManifestRejectsDuplicateAuthorityFields(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "duplicate publication field",
			data: `{
				"path":"publication/duplicate-publication-field",
				"functionOperations":[{"name":"sink","operation":{"effects":{"occurrences":[{"target":"effect-target","publication":{"kind":1,"kind":2}}],"tail":1}}}]
			}`,
		},
		{
			name: "case-folded duplicate publication field",
			data: `{
				"path":"publication/case-folded-duplicate-publication-field",
				"functionOperations":[{"name":"sink","operation":{"effects":{"occurrences":[{"target":"effect-target","publication":{"kind":1,"Kind":2}}],"tail":1}}}]
			}`,
		},
		{
			name: "duplicate occurrence field",
			data: `{
				"path":"publication/duplicate-occurrence-field",
				"functionOperations":[{"name":"sink","operation":{"effects":{"occurrences":[{"target":"effect-target","target":"other-target"}],"tail":1}}}]
			}`,
		},
		{
			name: "duplicate operation revision",
			data: `{
				"path":"publication/duplicate-operation-revision",
				"functionOperations":[{"name":"sink","operation":{"wireRevision":2,"wireRevision":0,"effects":{"occurrences":[{"target":"effect-target"}],"tail":1}}}]
			}`,
		},
		{
			name: "duplicate manifest revision",
			data: `{
				"path":"publication/duplicate-manifest-revision",
				"schemaRevision":2,
				"schemaRevision":0,
				"functionOperations":[{"name":"sink","operation":{"effects":{"occurrences":[{"target":"effect-target"}],"tail":1}}}]
			}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Decode([]byte(test.data)); err == nil || !strings.Contains(err.Error(), "duplicate JSON field") {
				t.Fatalf("Decode error = %v, want duplicate authority-field rejection", err)
			}
		})
	}
}

func TestPublicationEffectManifestRejectsSuperfluousRevisionMarkers(t *testing.T) {
	withoutOccurrences := []byte(`{
		"path":"publication/superfluous-manifest-revision",
		"schemaRevision":2
	}`)
	if _, err := Decode(withoutOccurrences); err == nil || !strings.Contains(err.Error(), "superfluous without effect occurrences") {
		t.Fatalf("Decode schema revision error = %v, want superfluous-marker rejection", err)
	}

	operationWithoutOccurrences := []byte(`{
		"path":"publication/superfluous-operation-revision",
		"functionOperations":[{"name":"sink","operation":{"wireRevision":2}}]
	}`)
	if _, err := Decode(operationWithoutOccurrences); err == nil || !strings.Contains(err.Error(), "superfluous without effect occurrences") {
		t.Fatalf("Decode operation revision error = %v, want superfluous-marker rejection", err)
	}
}
