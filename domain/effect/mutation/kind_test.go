package mutation

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
)

func TestKindOfTransformClassifiesValuePointerAndTypedNil(t *testing.T) {
	var nilUnchanged *Unchanged
	var nilElementUnion *ElementUnion
	var nilContainerElementUnion *ContainerElementUnion
	var nilToArray *ToArray

	tests := []struct {
		name    string
		value   TypeTransform
		pointer TypeTransform
		nilPtr  TypeTransform
		want    TransformKind
	}{
		{"unchanged", Unchanged{}, &Unchanged{}, nilUnchanged, TransformUnchanged},
		{"element union", ElementUnion{}, &ElementUnion{}, nilElementUnion, TransformElementUnion},
		{"container element union", ContainerElementUnion{}, &ContainerElementUnion{}, nilContainerElementUnion, TransformContainerElementUnion},
		{"to array", ToArray{}, &ToArray{}, nilToArray, TransformToArray},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindOfTransform(tt.value); got != tt.want {
				t.Fatalf("KindOfTransform(value) = %v, want %v", got, tt.want)
			}
			if got := KindOfTransform(tt.pointer); got != tt.want {
				t.Fatalf("KindOfTransform(pointer) = %v, want %v", got, tt.want)
			}
			if got := KindOfTransform(tt.nilPtr); got != tt.want {
				t.Fatalf("KindOfTransform(typed nil) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindOfTransformRejectsAbsentTransform(t *testing.T) {
	if got := KindOfTransform(nil); got != TransformUnknown {
		t.Fatalf("KindOfTransform(nil) = %v, want unknown", got)
	}
}

// TestTransformKindCatalogIsTheDenseEnumerationOfEveryValidKind states the
// density law a consumer's exhaustive iteration rests on: the catalog is every
// kind the admission predicate accepts, each once, in ordinal order from the
// first. A variant added to the type and not to the catalog is a verdict here
// rather than a variant a consumer silently never visits.
func TestTransformKindCatalogIsTheDenseEnumerationOfEveryValidKind(t *testing.T) {
	var admitted []TransformKind
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		if kind := TransformKind(candidate); kind.Valid() {
			admitted = append(admitted, kind)
		}
	}
	catalog := TransformKinds()
	if len(admitted) != TransformKindCount || len(catalog) != TransformKindCount {
		t.Fatalf("catalog holds %d kinds and the type admits %d, declared count is %d", len(catalog), len(admitted), TransformKindCount)
	}
	for position, kind := range catalog {
		if kind != admitted[position] {
			t.Fatalf("catalog position %d is kind %d, but the type's ordinal %d is kind %d", position, kind, position, admitted[position])
		}
		if int(kind) != position+1 {
			t.Fatalf("catalog position %d holds kind %d, so the ordinals are not dense from one", position, kind)
		}
	}
	if TransformUnknown.Valid() {
		t.Fatal("the absent kind was admitted as a declared member")
	}
}

// TestEveryDeclaredTransformKindIsInhabitedByATransform states the catalog is
// the vocabulary and not a list beside it: each declared kind names a transform
// the package can build, and that transform answers as its own kind.
func TestEveryDeclaredTransformKindIsInhabitedByATransform(t *testing.T) {
	samples := map[TransformKind]TypeTransform{
		TransformUnchanged:             Unchanged{},
		TransformElementUnion:          ElementUnion{},
		TransformContainerElementUnion: ContainerElementUnion{},
		TransformToArray:               ToArray{},
	}
	if len(samples) != TransformKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d are inhabited by a transform", TransformKindCount, len(samples))
	}
	for _, kind := range TransformKinds() {
		transform, sampled := samples[kind]
		if !sampled {
			t.Fatalf("declared kind %d names no transform of the vocabulary", kind)
		}
		if got := KindOfTransform(transform); got != kind {
			t.Fatalf("transform %T answers kind %d, not the kind %d it inhabits", transform, got, kind)
		}
	}
}

// TestTransformAccessorsNormalizeValueAndPointerSpellings states the ownership
// boundary the accessors hold: a transform reached by pointer is the same
// transform as the one reached by value, and a typed nil pointer is absent.
func TestTransformAccessorsNormalizeValueAndPointerSpellings(t *testing.T) {
	source := effect.ParamRef{Index: 1}
	container := effect.ParamRef{Index: 2}
	value := effect.ParamRef{Index: 3}
	element := effect.ParamRef{Index: 4}

	if got, ok := AsElementUnion(ElementUnion{Source: source}); !ok || got.Source != source {
		t.Fatalf("AsElementUnion(value) = %v/%v", got, ok)
	}
	if got, ok := AsElementUnion(&ElementUnion{Source: source}); !ok || got.Source != source {
		t.Fatalf("AsElementUnion(pointer) = %v/%v", got, ok)
	}
	if _, ok := AsElementUnion((*ElementUnion)(nil)); ok {
		t.Fatal("AsElementUnion admitted a typed nil pointer")
	}

	both := ContainerElementUnion{Container: container, Value: value}
	if got, ok := AsContainerElementUnion(both); !ok || got != both {
		t.Fatalf("AsContainerElementUnion(value) = %v/%v", got, ok)
	}
	if got, ok := AsContainerElementUnion(&both); !ok || got != both {
		t.Fatalf("AsContainerElementUnion(pointer) = %v/%v", got, ok)
	}
	if _, ok := AsContainerElementUnion((*ContainerElementUnion)(nil)); ok {
		t.Fatal("AsContainerElementUnion admitted a typed nil pointer")
	}

	if got, ok := AsToArray(ToArray{Element: element}); !ok || got.Element != element {
		t.Fatalf("AsToArray(value) = %v/%v", got, ok)
	}
	if got, ok := AsToArray(&ToArray{Element: element}); !ok || got.Element != element {
		t.Fatalf("AsToArray(pointer) = %v/%v", got, ok)
	}
	if _, ok := AsToArray((*ToArray)(nil)); ok {
		t.Fatal("AsToArray admitted a typed nil pointer")
	}

	if _, ok := AsUnchanged(Unchanged{}); !ok {
		t.Fatal("AsUnchanged rejected the value spelling")
	}
	if _, ok := AsUnchanged(&Unchanged{}); !ok {
		t.Fatal("AsUnchanged rejected the pointer spelling")
	}
	if _, ok := AsUnchanged((*Unchanged)(nil)); ok {
		t.Fatal("AsUnchanged admitted a typed nil pointer")
	}
}
