package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ExecutableRoot is an artifact-safe proof of one direct executable Body
// root. It carries only Flow's sealed semantic identity, family, and dense
// executable ordinal: neither the authored Term nor its Span may cross the
// ProgramArtifact boundary.
type ExecutableRoot struct {
	catalog *executableRootCatalog
	ordinal int
	id      identity.ContentID
	family  keyspace.Family
}

// ExecutableRoots is Program's complete dense artifact-safe root catalog for
// one Body. Construction fails closed when any Source denominator row cannot
// be joined to Flow; consumers never infer a denominator by silently skipping
// malformed source rows.
type ExecutableRoots struct {
	catalog *executableRootCatalog
}

type executableRootCatalog struct {
	body   Body
	rows   []ExecutableRoot
	sealed bool
}

func (roots ExecutableRoots) Available() bool {
	return roots.catalog != nil && roots.catalog.sealed && roots.catalog.body.Available()
}

func (roots ExecutableRoots) Count() int {
	if !roots.Available() {
		return 0
	}
	return len(roots.catalog.rows)
}

func (roots ExecutableRoots) At(index int) (ExecutableRoot, bool) {
	if !roots.Available() || index < 0 || index >= len(roots.catalog.rows) {
		return ExecutableRoot{}, false
	}
	root := roots.catalog.rows[index]
	return root, root.Available()
}

func (root ExecutableRoot) Available() bool {
	if root.catalog == nil || !root.catalog.sealed || !root.catalog.body.Available() || root.ordinal < 0 || !root.id.Available() || root.family == keyspace.FamilyInvalid || root.ordinal >= len(root.catalog.rows) {
		return false
	}
	issued := root.catalog.rows[root.ordinal]
	return issued.catalog == root.catalog && issued.ordinal == root.ordinal && issued.id == root.id && issued.family == root.family
}

func (root ExecutableRoot) ID() identity.ContentID {
	if !root.Available() {
		return identity.ContentID{}
	}
	return root.id
}

func (root ExecutableRoot) Family() keyspace.Family {
	if !root.Available() {
		return keyspace.FamilyInvalid
	}
	return root.family
}

func (body Body) ExecutableRoots() (ExecutableRoots, bool) {
	if !body.Available() {
		return ExecutableRoots{}, false
	}
	count, countOK := body.RootCount()
	if !countOK {
		return ExecutableRoots{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	if !bodyOK {
		return ExecutableRoots{}, false
	}
	catalog := &executableRootCatalog{body: body, rows: make([]ExecutableRoot, 0, count)}
	for sourceIndex := 0; sourceIndex < count; sourceIndex++ {
		authored, rootOK := body.program.Source().Index().BodyRootAt(bodyTerm, sourceIndex)
		if !rootOK {
			return ExecutableRoots{}, false
		}
		if !body.program.Flow().Executable().Contains(authored) {
			continue
		}
		id, idOK := body.program.Flow().SemanticTermPath(authored)
		root := ExecutableRoot{catalog: catalog, ordinal: len(catalog.rows), id: id, family: keyspace.TermFamily(authored)}
		if !idOK || !root.id.Available() || root.family == keyspace.FamilyInvalid {
			return ExecutableRoots{}, false
		}
		catalog.rows = append(catalog.rows, root)
	}
	catalog.sealed = true
	return ExecutableRoots{catalog: catalog}, true
}

func (input *Program) OwnsExecutableRoot(root ExecutableRoot) bool {
	return input.Available() && root.catalog != nil && root.catalog.body.program == input && input.OwnsBody(root.catalog.body) && root.Available()
}

func (input *Program) OwnsExecutableRoots(roots ExecutableRoots) bool {
	return input.Available() && roots.catalog != nil && roots.catalog.body.program == input && input.OwnsBody(roots.catalog.body) && roots.Available()
}
