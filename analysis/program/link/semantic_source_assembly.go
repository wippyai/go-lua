package link

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/relations"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
)

var (
	errSemanticSourceAssemblyUnavailable = errors.New("link: semantic-source assembly unavailable")
	errSemanticSourceAssemblySchema      = errors.New("link: semantic-source assembly schema")
	errSemanticSourceAssemblyFragment    = errors.New("link: semantic-source assembly fragment")
	errSemanticSourceAssemblyOverflow    = errors.New("link: semantic-source assembly count overflow")
)

// buildSemanticSourcePublications performs the one seal-time aggregate. Every
// owner contributes one detached publication set; Program mounts contribute
// the same 57-token set once per Project Shard, including duplicate mounts.
func buildSemanticSourcePublications(l *Link) (semanticsource.Publications, error) {
	if !l.sealedSemanticSource() {
		return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
	}
	project := l.Project()
	if project == nil {
		return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
	}
	schema, err := relations.CanonicalSchema()
	if err != nil {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	assembly, err := newSemanticSourceAssembly(schema)
	if err != nil {
		return semanticsource.Publications{}, err
	}
	mounts := project.Mounts()
	for index := 0; index < mounts.Count(); index++ {
		shard, ok := mounts.At(index)
		if !ok {
			return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
		}
		p, ok := mounts.Program(shard)
		if !ok || p == nil {
			return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
		}
		publications := p.SemanticSourcePublications(schema)
		if err := assembly.acceptProgram(publications); err != nil {
			return semanticsource.Publications{}, err
		}
	}
	contract, ok := l.boundary.Target()
	if !ok || contract == nil {
		return semanticsource.Publications{}, errSemanticSourceAssemblyUnavailable
	}
	targetViews, ok := contract.SourceViews()
	if !ok || targetViews.OwnerID() != contract.ContentID() {
		return semanticsource.Publications{}, errSemanticSourceAssemblyFragment
	}
	if err := assembly.acceptOne(semanticSourceTarget, targetViews.Publications(schema)); err != nil {
		return semanticsource.Publications{}, err
	}
	linkRows, ok := l.childSourcePublications(schema)
	if !ok {
		return semanticsource.Publications{}, errSemanticSourceAssemblyFragment
	}
	if err := assembly.acceptLink(linkRows); err != nil {
		return semanticsource.Publications{}, err
	}
	publications, err := assembly.seal()
	if err != nil || publications.SchemaDigest() != schema.SchemaDigest() {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	return publications, nil
}

type semanticSourceContributor uint8

const (
	semanticSourceContributorUnset semanticSourceContributor = iota
	semanticSourceProgram
	semanticSourceTarget
	semanticSourceProject
	semanticSourceBoundary
	semanticSourceModule
	semanticSourceStatic
	semanticSourceHost
)

const semanticSourceContributorCount = int(semanticSourceHost) + 1

type semanticSourceExpectation struct {
	definition  semanticsource.RelationDef
	contributor semanticSourceContributor
}

// semanticSourceAssembly is a schema-indexed cardinality verifier. The
// canonical relation schema is the only token authority; no generic token
// registry, relation copy, or owner adapter is retained.
type semanticSourceAssembly struct {
	schema         *relations.Schema
	rows           []relations.Row
	schemaExpected []semanticSourceExpectation
	expectedCount  [semanticSourceContributorCount]int
	totals         []int
	counted        []bool
	accepted       [semanticSourceContributorCount]bool
}

func newSemanticSourceAssembly(schema *relations.Schema) (*semanticSourceAssembly, error) {
	if schema == nil || schema.Count() == 0 || !schema.SchemaDigest().Available() {
		return nil, errSemanticSourceAssemblySchema
	}
	rows := schema.Rows()
	assembly := &semanticSourceAssembly{schema: schema, rows: rows, schemaExpected: make([]semanticSourceExpectation, len(rows)), totals: make([]int, len(rows)), counted: make([]bool, len(rows))}
	for index, row := range rows {
		contributor, ok := semanticSourceContributorFor(row.Owner)
		if !ok {
			return nil, errSemanticSourceAssemblySchema
		}
		token := row.Definition.Token()
		definition, defined := schema.Definition(token.Origin(), token.Facet())
		if token == (semanticsource.Token{}) || !defined || row.Definition != definition {
			return nil, errSemanticSourceAssemblySchema
		}
		assembly.schemaExpected[index] = semanticSourceExpectation{definition: definition, contributor: contributor}
		assembly.expectedCount[contributor]++
	}
	for contributor := semanticSourceProgram; contributor <= semanticSourceHost; contributor++ {
		if assembly.expectedCount[contributor] == 0 {
			return nil, errSemanticSourceAssemblySchema
		}
	}
	if len(rows) != schema.Count() {
		return nil, errSemanticSourceAssemblySchema
	}
	return assembly, nil
}

func semanticSourceContributorFor(owner relations.Owner) (semanticSourceContributor, bool) {
	switch owner {
	case relations.OwnerProgramSource, relations.OwnerProgramFlow, relations.OwnerProgramStatic, relations.OwnerProgramModule:
		return semanticSourceProgram, true
	case relations.OwnerTarget:
		return semanticSourceTarget, true
	case relations.OwnerLinkProject:
		return semanticSourceProject, true
	case relations.OwnerLinkBoundary:
		return semanticSourceBoundary, true
	case relations.OwnerLinkModule:
		return semanticSourceModule, true
	case relations.OwnerLinkStatic:
		return semanticSourceStatic, true
	case relations.OwnerLinkHost:
		return semanticSourceHost, true
	default:
		return semanticSourceContributorUnset, false
	}
}
func (assembly *semanticSourceAssembly) indexOf(definition semanticsource.RelationDef) (int, bool) {
	if assembly == nil {
		return 0, false
	}
	token := definition.Token()
	for index, expected := range assembly.schemaExpected {
		if expected.definition == definition && assembly.rows[index].Definition.Token() == token {
			return index, true
		}
	}
	return 0, false
}
func (assembly *semanticSourceAssembly) acceptProgram(publications []semanticsource.Publication) error {
	if assembly == nil {
		return errSemanticSourceAssemblyFragment
	}
	return assembly.acceptSum(semanticSourceProgram, publications)
}
func (assembly *semanticSourceAssembly) acceptOne(contributor semanticSourceContributor, publications []semanticsource.Publication) error {
	if assembly == nil || contributor == semanticSourceContributorUnset || contributor == semanticSourceProgram || int(contributor) >= semanticSourceContributorCount || assembly.accepted[contributor] {
		return errSemanticSourceAssemblyFragment
	}
	seen := make([]bool, len(assembly.schemaExpected))
	for _, publication := range publications {
		index, ok := assembly.indexOf(publication.Definition())
		if !ok || assembly.schemaExpected[index].contributor != contributor || publication.Count() < 0 || seen[index] {
			return errSemanticSourceAssemblyFragment
		}
		seen[index] = true
		assembly.totals[index] = publication.Count()
		assembly.counted[index] = true
	}
	for index, expected := range assembly.schemaExpected {
		if expected.contributor == contributor && !seen[index] {
			return errSemanticSourceAssemblyFragment
		}
	}
	assembly.accepted[contributor] = true
	return nil
}
func (assembly *semanticSourceAssembly) acceptSum(contributor semanticSourceContributor, publications []semanticsource.Publication) error {
	if assembly == nil || contributor != semanticSourceProgram || len(publications) != assembly.expectedCount[contributor] {
		return errSemanticSourceAssemblyFragment
	}
	seen := make([]bool, len(assembly.schemaExpected))
	for _, publication := range publications {
		index, ok := assembly.indexOf(publication.Definition())
		if !ok || assembly.schemaExpected[index].contributor != contributor || publication.Count() < 0 || seen[index] {
			return errSemanticSourceAssemblyFragment
		}
		seen[index] = true
		if publication.Count() > maxInt()-assembly.totals[index] {
			return errSemanticSourceAssemblyOverflow
		}
		assembly.totals[index] += publication.Count()
		assembly.counted[index] = true
	}
	for index, expected := range assembly.schemaExpected {
		if expected.contributor == contributor && !seen[index] {
			return errSemanticSourceAssemblyFragment
		}
	}
	return nil
}
func (assembly *semanticSourceAssembly) acceptLink(publications []semanticsource.Publication) error {
	linkCount := 0
	if assembly != nil {
		for contributor := semanticSourceProject; contributor <= semanticSourceHost; contributor++ {
			linkCount += assembly.expectedCount[contributor]
		}
	}
	if assembly == nil || len(publications) != linkCount || assembly.accepted[semanticSourceProject] || assembly.accepted[semanticSourceBoundary] || assembly.accepted[semanticSourceModule] || assembly.accepted[semanticSourceStatic] || assembly.accepted[semanticSourceHost] {
		return errSemanticSourceAssemblyFragment
	}
	seen := make([]bool, len(assembly.schemaExpected))
	counts := [semanticSourceContributorCount]int{}
	for _, publication := range publications {
		index, ok := assembly.indexOf(publication.Definition())
		if !ok || publication.Count() < 0 || seen[index] {
			return errSemanticSourceAssemblyFragment
		}
		contributor := assembly.schemaExpected[index].contributor
		if contributor < semanticSourceProject || contributor > semanticSourceHost {
			return errSemanticSourceAssemblyFragment
		}
		seen[index] = true
		assembly.totals[index] = publication.Count()
		assembly.counted[index] = true
		counts[contributor]++
	}
	for index, expected := range assembly.schemaExpected {
		if expected.contributor >= semanticSourceProject && !seen[index] {
			return errSemanticSourceAssemblyFragment
		}
	}
	for contributor := semanticSourceProject; contributor <= semanticSourceHost; contributor++ {
		if counts[contributor] != assembly.expectedCount[contributor] {
			return errSemanticSourceAssemblyFragment
		}
		assembly.accepted[contributor] = true
	}
	return nil
}
func (assembly *semanticSourceAssembly) seal() (semanticsource.Publications, error) {
	if assembly == nil || len(assembly.schemaExpected) == 0 {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	for index := range assembly.schemaExpected {
		if !assembly.counted[index] {
			return semanticsource.Publications{}, errSemanticSourceAssemblyFragment
		}
	}
	publisher, err := semanticsource.NewPublisher(assembly.schema)
	if err != nil {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	for index := 0; index < len(assembly.rows); index++ {
		definition := assembly.rows[index].Definition
		expectedIndex, ok := assembly.indexOf(definition)
		if !ok || !assembly.counted[expectedIndex] {
			return semanticsource.Publications{}, errSemanticSourceAssemblySchema
		}
		publication, err := semanticsource.SealPublication(definition, assembly.totals[expectedIndex])
		if err != nil || publisher.Accept(publication) != nil {
			return semanticsource.Publications{}, errSemanticSourceAssemblySchema
		}
	}
	publications, err := publisher.Seal()
	if err != nil {
		return semanticsource.Publications{}, errSemanticSourceAssemblySchema
	}
	return publications, nil
}

func maxInt() int { return int(^uint(0) >> 1) }
