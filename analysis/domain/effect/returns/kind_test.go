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
