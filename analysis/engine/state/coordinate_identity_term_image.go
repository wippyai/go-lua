package state

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

type CoordinateIdentityTermBinding struct {
	Source identity.Term
	Images []identity.Term
}

// CoordinateIdentityTermImage is the immutable set-valued topology image of
// an Apply. The executable flat identity lattice may be Top while this syntax
// support still names every exact correlated alternative.
type CoordinateIdentityTermImage struct {
	bindings map[identity.Term][]identity.Term
}

func NewCoordinateIdentityTermImage(bindings []CoordinateIdentityTermBinding) (*CoordinateIdentityTermImage, bool) {
	out := &CoordinateIdentityTermImage{bindings: make(map[identity.Term][]identity.Term, len(bindings))}
	for _, binding := range bindings {
		if _, formal := binding.Source.Formal(); !formal {
			return nil, false
		}
		if _, duplicate := out.bindings[binding.Source]; duplicate {
			return nil, false
		}
		seen := make(map[identity.Term]struct{}, len(binding.Images))
		images := make([]identity.Term, 0, len(binding.Images))
		for _, term := range binding.Images {
			if !term.Valid() {
				return nil, false
			}
			if _, exists := seen[term]; !exists {
				seen[term] = struct{}{}
				images = append(images, term)
			}
		}
		sort.Slice(images, func(i, j int) bool { return identityTermLess(images[i], images[j]) })
		out.bindings[binding.Source] = images
	}
	return out, true
}

// Bindings returns a detached canonical spelling of the finite image. Sources
// and their image sets use the state domain's structural identity order; map
// iteration order can therefore never enter a frozen boundary relation.
func (i *CoordinateIdentityTermImage) Bindings() []CoordinateIdentityTermBinding {
	if i == nil {
		return nil
	}
	sources := make([]identity.Term, 0, len(i.bindings))
	for source := range i.bindings {
		sources = append(sources, source)
	}
	sort.Slice(sources, func(left, right int) bool { return identityTermLess(sources[left], sources[right]) })
	out := make([]CoordinateIdentityTermBinding, len(sources))
	for index, source := range sources {
		out[index] = CoordinateIdentityTermBinding{
			Source: source,
			Images: append([]identity.Term(nil), i.bindings[source]...),
		}
	}
	return out
}

// Equal reports exact equality of two canonical finite images. Nil denotes
// absence of identity-image semantics and is deliberately distinct from a
// present image with no bindings.
func (i *CoordinateIdentityTermImage) Equal(other *CoordinateIdentityTermImage) bool {
	if i == nil || other == nil {
		return i == nil && other == nil
	}
	if len(i.bindings) != len(other.bindings) {
		return false
	}
	for source, images := range i.bindings {
		right, ok := other.bindings[source]
		if !ok || !identityTermSlicesEqual(images, right) {
			return false
		}
	}
	return true
}

// Union returns the immutable canonical pointwise union. Both operands must
// be present images; nil is not treated as the empty relation because it means
// that the enclosing boundary plan is not using identity-image semantics.
func (i *CoordinateIdentityTermImage) Union(other *CoordinateIdentityTermImage) (*CoordinateIdentityTermImage, bool) {
	if i == nil || other == nil {
		return nil, false
	}
	bindings := make(map[identity.Term][]identity.Term, len(i.bindings)+len(other.bindings))
	for source, images := range i.bindings {
		bindings[source] = append([]identity.Term(nil), images...)
	}
	for source, images := range other.bindings {
		bindings[source] = unionIdentityTermSlices(bindings[source], images)
	}
	return coordinateIdentityTermImageFromCanonical(bindings), true
}

// Delta returns the exact image alternatives added since prior. False means
// the receiver is not a monotone extension: a source binding or one of its
// alternatives was removed. Empty newly-declared bindings are retained by the
// receiver but do not appear in the delta because they wake no source slot.
func (i *CoordinateIdentityTermImage) Delta(prior *CoordinateIdentityTermImage) (*CoordinateIdentityTermImage, bool) {
	if i == nil || prior == nil {
		return nil, false
	}
	for source, previous := range prior.bindings {
		current, ok := i.bindings[source]
		if !ok || !identityTermSliceContainsAll(current, previous) {
			return nil, false
		}
	}
	added := make(map[identity.Term][]identity.Term)
	for source, current := range i.bindings {
		delta := subtractIdentityTermSlices(current, prior.bindings[source])
		if len(delta) != 0 {
			added[source] = delta
		}
	}
	return coordinateIdentityTermImageFromCanonical(added), true
}

func (i *CoordinateIdentityTermImage) Image(source identity.Term) ([]identity.Term, bool) {
	if i == nil || !source.Valid() {
		return nil, false
	}
	if _, formal := source.Formal(); !formal {
		return []identity.Term{source}, true
	}
	terms, ok := i.bindings[source]
	return append([]identity.Term(nil), terms...), ok
}

func coordinateIdentityTermImageFromCanonical(bindings map[identity.Term][]identity.Term) *CoordinateIdentityTermImage {
	out := &CoordinateIdentityTermImage{bindings: make(map[identity.Term][]identity.Term, len(bindings))}
	for source, images := range bindings {
		out.bindings[source] = append([]identity.Term(nil), images...)
	}
	return out
}

func identityTermSlicesEqual(left, right []identity.Term) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func identityTermSliceContainsAll(whole, subset []identity.Term) bool {
	for wholeIndex, subsetIndex := 0, 0; subsetIndex < len(subset); {
		for wholeIndex < len(whole) && identityTermLess(whole[wholeIndex], subset[subsetIndex]) {
			wholeIndex++
		}
		if wholeIndex == len(whole) || whole[wholeIndex] != subset[subsetIndex] {
			return false
		}
		wholeIndex++
		subsetIndex++
	}
	return true
}

func unionIdentityTermSlices(left, right []identity.Term) []identity.Term {
	out := make([]identity.Term, 0, len(left)+len(right))
	for leftIndex, rightIndex := 0, 0; leftIndex < len(left) || rightIndex < len(right); {
		switch {
		case leftIndex == len(left):
			out = append(out, right[rightIndex:]...)
			return out
		case rightIndex == len(right):
			out = append(out, left[leftIndex:]...)
			return out
		case left[leftIndex] == right[rightIndex]:
			out = append(out, left[leftIndex])
			leftIndex++
			rightIndex++
		case identityTermLess(left[leftIndex], right[rightIndex]):
			out = append(out, left[leftIndex])
			leftIndex++
		default:
			out = append(out, right[rightIndex])
			rightIndex++
		}
	}
	return out
}

func subtractIdentityTermSlices(whole, remove []identity.Term) []identity.Term {
	out := make([]identity.Term, 0, len(whole))
	for wholeIndex, removeIndex := 0, 0; wholeIndex < len(whole); wholeIndex++ {
		for removeIndex < len(remove) && identityTermLess(remove[removeIndex], whole[wholeIndex]) {
			removeIndex++
		}
		if removeIndex == len(remove) || remove[removeIndex] != whole[wholeIndex] {
			out = append(out, whole[wholeIndex])
		}
	}
	return out
}
