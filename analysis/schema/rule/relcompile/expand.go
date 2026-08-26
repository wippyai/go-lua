package relcompile

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// InstallMemberCatalog installs the sealed owner vocabulary used to validate
// an Expand declaration. The catalog is cold composition input and is never
// retained by an expression or consulted by runtime.
func (registry *Registry) InstallMemberCatalog(axis schema.EntryReference, catalog member.Catalog) error {
	site := Site{Path: "registry.member-catalog"}
	axisName := Name{Entry: axis}
	if !axis.Available() {
		return refuse(site, axisName, KindOwner, ReasonUnavailable)
	}
	if _, known := registry.owners[axis]; !known {
		return refuse(site, axisName, KindOwner, ReasonUnknown)
	}
	if !catalog.Available() {
		return refuse(site, axisName, KindRelation, ReasonUnavailable)
	}
	if _, duplicate := registry.memberCatalogs[axis]; duplicate {
		return refuse(site, axisName, KindOwner, ReasonDuplicateName)
	}
	registry.memberCatalogs[axis] = catalog.Clone()
	return nil
}

// ExpandContract resolves one authored KeyVector declaration into the one
// model-owned dependent-join contract. The registry consults owner catalogs
// only at seal; it does not retain a span enum, a coordinate vector, or a
// physical ordinal. Correlation is represented by the owner-issued provider
// relation identity itself. The owner-authored vector order and sparse extent
// are evidence, not logical contract fields.
func (registry *Registry) ExpandContract(site Site, candidate, publisher, reader, key Name, scope model.ScopeID) (model.ExpandContract, error) {
	candidateID, err := registry.Relation(site, candidate)
	if err != nil {
		return model.ExpandContract{}, err
	}
	publisherID, err := registry.Relation(site, publisher)
	if err != nil {
		return model.ExpandContract{}, err
	}
	readerID, err := registry.Relation(site, reader)
	if err != nil {
		return model.ExpandContract{}, err
	}
	keyID, err := registry.Column(site, key)
	if err != nil {
		return model.ExpandContract{}, err
	}
	if keyID.Relation() != readerID {
		return model.ExpandContract{}, refuse(site, key, KindExpand, ReasonForeign)
	}
	if !scope.Available() {
		return model.ExpandContract{}, refuse(site, reader, KindScope, ReasonUnavailable)
	}

	publisherCatalog, ok := registry.memberCatalogs[publisher.Entry]
	if !ok || !publisherCatalog.Available() {
		return model.ExpandContract{}, refuse(site, publisher, KindExpand, ReasonUndeclared)
	}
	publisherRelation, ok := publisherCatalog.Relation(publisher.Member)
	if !ok || !publisherRelation.PublishesKeyVector {
		return model.ExpandContract{}, refuse(site, publisher, KindExpand, ReasonUndeclared)
	}
	readerCatalog, ok := registry.memberCatalogs[reader.Entry]
	if !ok || !readerCatalog.Available() {
		return model.ExpandContract{}, refuse(site, reader, KindExpand, ReasonUndeclared)
	}
	readerRelation, ok := readerCatalog.Relation(reader.Member)
	if !ok || !readerRelation.CandidateProvider.Available() || !readerRelation.CandidateProvider.AxisRelation.Available() {
		return model.ExpandContract{}, refuse(site, reader, KindExpand, ReasonUndeclared)
	}

	providerRef := readerRelation.CandidateProvider.AxisRelation
	provider := NewName(providerRef.Axis, providerRef.Member)
	providerID, err := registry.Relation(site, provider)
	if err != nil {
		return model.ExpandContract{}, err
	}
	declared := member.RelationRef{Axis: publisher.Entry, Member: publisher.Member}
	if providerRef != declared {
		corresponded := false
		for _, relation := range publisherRelation.Correspondences {
			if relation == providerRef {
				corresponded = true
				break
			}
		}
		if !corresponded {
			return model.ExpandContract{}, refuse(site, publisher, KindExpand, ReasonForeign)
		}
	}

	contract := model.DefineExpandContract(candidateID, publisherID, readerID, keyID, providerID)
	contract = contract.WithScope(scope)
	if !contract.Available() {
		return model.ExpandContract{}, refuse(site, publisher, KindExpand, ReasonUnavailable)
	}
	return contract, nil
}
