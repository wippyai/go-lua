package interproc

import (
	"fmt"
	"sort"
)

// Dependency is one direct content-identity edge. Kind names the authority
// class (for example source, provider, registry, domain, contract, solver, or
// codec); it is not a mutable version or a path.
type Dependency struct {
	Kind string
	ID   ContentID
}

func (d Dependency) valid() bool { return d.Kind != "" && d.ID.Valid() }
func (d Dependency) less(other Dependency) bool {
	if d.Kind != other.Kind {
		return d.Kind < other.Kind
	}
	return string(d.ID[:]) < string(other.ID[:])
}

// DependencyManifest is the direct content chain captured by an artifact.
// Later invalidation stages consume these exact edges; it has no generation
// field because generations are never semantic cache identity.
type DependencyManifest struct{ dependencies []Dependency }

func NewDependencyManifest(dependencies []Dependency) (DependencyManifest, error) {
	out := DependencyManifest{dependencies: append([]Dependency(nil), dependencies...)}
	sort.Slice(out.dependencies, func(i, j int) bool { return out.dependencies[i].less(out.dependencies[j]) })
	for index, dependency := range out.dependencies {
		if !dependency.valid() {
			return DependencyManifest{}, fmt.Errorf("interproc: malformed content dependency")
		}
		if index != 0 && !out.dependencies[index-1].less(dependency) {
			return DependencyManifest{}, fmt.Errorf("interproc: duplicate content dependency %q", dependency.Kind)
		}
	}
	return out, nil
}

func (m DependencyManifest) Valid() bool { return m.CanonicalBytes() != nil }
func (m DependencyManifest) Dependencies() []Dependency {
	return append([]Dependency(nil), m.dependencies...)
}
func (m DependencyManifest) CanonicalBytes() []byte {
	if len(m.dependencies) == 0 {
		return nil
	}
	for index, dependency := range m.dependencies {
		if !dependency.valid() || index != 0 && !m.dependencies[index-1].less(dependency) {
			return nil
		}
	}
	out := appendText(nil, "interproc-dependency-manifest/content-v1")
	out = appendU64(out, uint64(len(m.dependencies)))
	for _, dependency := range m.dependencies {
		out = appendText(out, dependency.Kind)
		out = append(out, dependency.ID[:]...)
	}
	return out
}
func (m DependencyManifest) ContentID() ContentID {
	encoded := m.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}
