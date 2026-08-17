package returns

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect/capability"
)

func TestKindOfReturnTypeClassifiesValueAndPointerSpellings(t *testing.T) {
	tests := []struct {
		name    string
		value   ReturnType
		pointer ReturnType
		want    ReturnTypeKind
	}{
		{"same as", SameAs{}, &SameAs{}, ReturnTypeSameAs},
		{"element of", ElementOf{}, &ElementOf{}, ReturnTypeElementOf},
		{"optional element of", OptionalElementOf{}, &OptionalElementOf{}, ReturnTypeOptionalElementOf},
		{"callback", CallbackReturn{}, &CallbackReturn{}, ReturnTypeCallbackReturn},
		{"array callback", ArrayOfCallbackReturn{}, &ArrayOfCallbackReturn{}, ReturnTypeArrayOfCallbackReturn},
		{"type projection", TypeProjection{}, &TypeProjection{}, ReturnTypeTypeProjection},
		{"conditional type", ConditionalType{}, &ConditionalType{}, ReturnTypeConditionalType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindOfReturnType(tt.value); got != tt.want {
				t.Fatalf("KindOfReturnType(value) = %v, want %v", got, tt.want)
			}
			if got := KindOfReturnType(tt.pointer); got != tt.want {
				t.Fatalf("KindOfReturnType(pointer) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindOfReturnTypeRejectsAbsentTransform(t *testing.T) {
	if got := KindOfReturnType(nil); got != ReturnTypeUnknown {
		t.Fatalf("KindOfReturnType(nil) = %v, want unknown", got)
	}
}

// typedNilTransforms is one typed nil pointer per vocabulary member: a term
// whose Go type names a variant while carrying no transform.
func typedNilTransforms() map[ReturnTypeKind]ReturnType {
	return map[ReturnTypeKind]ReturnType{
		ReturnTypeSameAs:                (*SameAs)(nil),
		ReturnTypeElementOf:             (*ElementOf)(nil),
		ReturnTypeOptionalElementOf:     (*OptionalElementOf)(nil),
		ReturnTypeCallbackReturn:        (*CallbackReturn)(nil),
		ReturnTypeArrayOfCallbackReturn: (*ArrayOfCallbackReturn)(nil),
		ReturnTypeTypeProjection:        (*TypeProjection)(nil),
		ReturnTypeConditionalType:       (*ConditionalType)(nil),
	}
}

// TestTypedNilTransformIsAbsentThroughEveryAuthority states the package's one
// reading of a typed nil pointer: it names a variant but carries no transform,
// so every authority that reads one answers absent. The classifier answers the
// absent kind, the kind states no capability, the label carrying it states none
// either, and the concrete accessor yields nothing. Coverage is derived from the
// vocabulary catalog, so a variant added without a typed nil term is a verdict
// here rather than a spelling one authority reads differently from the rest.
func TestTypedNilTransformIsAbsentThroughEveryAuthority(t *testing.T) {
	absentTerms := typedNilTransforms()
	if len(absentTerms) != ReturnTypeKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d have a typed nil term", ReturnTypeKindCount, len(absentTerms))
	}
	for _, kind := range ReturnTypeKinds() {
		absent, termed := absentTerms[kind]
		if !termed {
			t.Fatalf("declared kind %d has no typed nil term", kind)
		}
		if !IsNilReturnType(absent) {
			t.Fatalf("typed nil %T is not read as absent", absent)
		}
		if got := KindOfReturnType(absent); got != ReturnTypeUnknown {
			t.Fatalf("KindOfReturnType(typed nil %T) = %v, want %v", absent, got, ReturnTypeUnknown)
		}
		if got := KindOfReturnType(absent).CapabilityID(); got != "" {
			t.Fatalf("typed nil %T states capability %q, want none", absent, got)
		}
		if got := (Return{ReturnIndex: 0, Transform: absent}).CapabilityID(); got != "" {
			t.Fatalf("return carrying typed nil %T states capability %q, want none", absent, got)
		}
		if !returnTypeEquals(absent, nil) {
			t.Fatalf("typed nil %T does not equal an absent transform", absent)
		}
	}
}

// TestPresentTransformStatesItsOwnCapability is the mutation check on the law
// above: making absence the answer for every typed nil must not make it the
// answer for a transform that is present, in either spelling.
func TestPresentTransformStatesItsOwnCapability(t *testing.T) {
	present := map[ReturnTypeKind][2]ReturnType{
		ReturnTypeSameAs:                {SameAs{}, &SameAs{}},
		ReturnTypeElementOf:             {ElementOf{}, &ElementOf{}},
		ReturnTypeOptionalElementOf:     {OptionalElementOf{}, &OptionalElementOf{}},
		ReturnTypeCallbackReturn:        {CallbackReturn{}, &CallbackReturn{}},
		ReturnTypeArrayOfCallbackReturn: {ArrayOfCallbackReturn{}, &ArrayOfCallbackReturn{}},
		ReturnTypeTypeProjection:        {TypeProjection{}, &TypeProjection{}},
		ReturnTypeConditionalType:       {ConditionalType{}, &ConditionalType{}},
	}
	if len(present) != ReturnTypeKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d are inhabited here", ReturnTypeKindCount, len(present))
	}
	for _, kind := range ReturnTypeKinds() {
		spellings, inhabited := present[kind]
		if !inhabited {
			t.Fatalf("declared kind %d names no present transform", kind)
		}
		for _, transform := range spellings {
			if IsNilReturnType(transform) {
				t.Fatalf("present transform %T is read as absent", transform)
			}
			if got := KindOfReturnType(transform); got != kind {
				t.Fatalf("KindOfReturnType(%T) = %v, want %v", transform, got, kind)
			}
			if got := (Return{ReturnIndex: 0, Transform: transform}).CapabilityID(); got != kind.CapabilityID() {
				t.Fatalf("return carrying %T states capability %q, want %q", transform, got, kind.CapabilityID())
			}
		}
	}
}

// TestReturnTypeKindCatalogIsTheDenseEnumerationOfEveryValidKind states the
// density law a consumer's exhaustive iteration rests on: the catalog is every
// kind the admission predicate accepts, each once, in ordinal order from the
// first. A variant added to the type and not to the catalog is a verdict here
// rather than a variant a consumer silently never visits.
func TestReturnTypeKindCatalogIsTheDenseEnumerationOfEveryValidKind(t *testing.T) {
	var admitted []ReturnTypeKind
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		if kind := ReturnTypeKind(candidate); kind.Valid() {
			admitted = append(admitted, kind)
		}
	}
	catalog := ReturnTypeKinds()
	if len(admitted) != ReturnTypeKindCount || len(catalog) != ReturnTypeKindCount {
		t.Fatalf("catalog holds %d kinds and the type admits %d, declared count is %d", len(catalog), len(admitted), ReturnTypeKindCount)
	}
	for position, kind := range catalog {
		if kind != admitted[position] {
			t.Fatalf("catalog position %d is kind %d, but the type's ordinal %d is kind %d", position, kind, position, admitted[position])
		}
		if int(kind) != position+1 {
			t.Fatalf("catalog position %d holds kind %d, so the ordinals are not dense from one", position, kind)
		}
	}
	if ReturnTypeUnknown.Valid() {
		t.Fatal("the absent kind was admitted as a declared member")
	}
}

// TestEveryDeclaredReturnTypeKindIsInhabitedByATransform states the catalog is
// the vocabulary and not a list beside it: each declared kind names a transform
// the package can build, and that transform answers as its own kind.
func TestEveryDeclaredReturnTypeKindIsInhabitedByATransform(t *testing.T) {
	samples := map[ReturnTypeKind]ReturnType{
		ReturnTypeSameAs:                SameAs{},
		ReturnTypeElementOf:             ElementOf{},
		ReturnTypeOptionalElementOf:     OptionalElementOf{},
		ReturnTypeCallbackReturn:        CallbackReturn{},
		ReturnTypeArrayOfCallbackReturn: ArrayOfCallbackReturn{},
		ReturnTypeTypeProjection:        TypeProjection{},
		ReturnTypeConditionalType:       ConditionalType{},
	}
	if len(samples) != ReturnTypeKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d are inhabited by a transform", ReturnTypeKindCount, len(samples))
	}
	for _, kind := range ReturnTypeKinds() {
		transform, sampled := samples[kind]
		if !sampled {
			t.Fatalf("declared kind %d names no transform of the vocabulary", kind)
		}
		if got := KindOfReturnType(transform); got != kind {
			t.Fatalf("transform %T answers kind %d, not the kind %d it inhabits", transform, got, kind)
		}
	}
}

// TestEveryReturnTypeKindStatesItsCapability states that the capability
// classification rides the kind catalog: every member of the closed vocabulary
// names an audited capability, no two members name the same one, and a kind
// outside the vocabulary names none.
func TestEveryReturnTypeKindStatesItsCapability(t *testing.T) {
	seen := map[string]ReturnTypeKind{}
	for _, kind := range ReturnTypeKinds() {
		id := kind.CapabilityID()
		if id == "" {
			t.Fatalf("kind %d states no capability", kind)
		}
		if _, known := capability.Lookup(id); !known {
			t.Fatalf("kind %d states unaudited capability %s", kind, id)
		}
		if previous, ok := seen[id]; ok {
			t.Fatalf("%s is stated by kinds %d and %d", id, previous, kind)
		}
		seen[id] = kind
	}
	if len(seen) != ReturnTypeKindCount {
		t.Fatalf("capabilities stated = %d, kinds = %d", len(seen), ReturnTypeKindCount)
	}
	if got := ReturnTypeUnknown.CapabilityID(); got != "" {
		t.Fatalf("ReturnTypeUnknown states %q, want no capability", got)
	}
}
