package returns

import "testing"

func TestKindOfReturnTypeClassifiesValuePointerAndTypedNil(t *testing.T) {
	var nilSame *SameAs
	var nilElement *ElementOf
	var nilOptional *OptionalElementOf
	var nilCallback *CallbackReturn
	var nilArrayCallback *ArrayOfCallbackReturn
	var nilProjection *TypeProjection
	var nilConditional *ConditionalType

	tests := []struct {
		name    string
		value   ReturnType
		pointer ReturnType
		nilPtr  ReturnType
		want    ReturnTypeKind
	}{
		{"same as", SameAs{}, &SameAs{}, nilSame, ReturnTypeSameAs},
		{"element of", ElementOf{}, &ElementOf{}, nilElement, ReturnTypeElementOf},
		{"optional element of", OptionalElementOf{}, &OptionalElementOf{}, nilOptional, ReturnTypeOptionalElementOf},
		{"callback", CallbackReturn{}, &CallbackReturn{}, nilCallback, ReturnTypeCallbackReturn},
		{"array callback", ArrayOfCallbackReturn{}, &ArrayOfCallbackReturn{}, nilArrayCallback, ReturnTypeArrayOfCallbackReturn},
		{"type projection", TypeProjection{}, &TypeProjection{}, nilProjection, ReturnTypeTypeProjection},
		{"conditional type", ConditionalType{}, &ConditionalType{}, nilConditional, ReturnTypeConditionalType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindOfReturnType(tt.value); got != tt.want {
				t.Fatalf("KindOfReturnType(value) = %v, want %v", got, tt.want)
			}
			if got := KindOfReturnType(tt.pointer); got != tt.want {
				t.Fatalf("KindOfReturnType(pointer) = %v, want %v", got, tt.want)
			}
			if got := KindOfReturnType(tt.nilPtr); got != tt.want {
				t.Fatalf("KindOfReturnType(typed nil) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindOfReturnTypeRejectsAbsentTransform(t *testing.T) {
	if got := KindOfReturnType(nil); got != ReturnTypeUnknown {
		t.Fatalf("KindOfReturnType(nil) = %v, want unknown", got)
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
