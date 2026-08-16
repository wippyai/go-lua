package boundary

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

const contentVersion = 5

// Build validates the exact Project/Target authorities and derives the one
// scoped require operation. It does not enumerate or retain Application x
// Operation membership.
func Build(input Input) (*Draft, error) {
	if input.Project == nil {
		return nil, errors.New("link/boundary: nil Project")
	}
	if input.Target == nil || !input.Target.ContentID().Available() {
		return nil, errors.New("link/boundary: unavailable Target")
	}
	if !input.Project.MatchesTarget(input.Target) {
		return nil, errors.New("link/boundary: Project/Target authority mismatch")
	}
	relation, ok := input.Project.ApplicationRelationID()
	if !ok {
		return nil, errors.New("link/boundary: unavailable Application relation")
	}
	require, err := scopedRequireOperation(input.Target)
	if err != nil {
		return nil, err
	}
	authority := &authority{project: input.Project, target: input.Target, require: require}
	if err := sealValues(authority); err != nil {
		return nil, err
	}
	if err := sealMountedCalls(authority); err != nil {
		return nil, err
	}
	authority.moduleRelation = moduleRelationID(authority.valueTable.content, input.Target, require)
	if !authority.moduleRelation.Available() {
		return nil, errors.New("link/boundary: unavailable module relation identity")
	}
	if err := sealSeeds(authority, input.EndpointRequests); err != nil {
		return nil, err
	}
	content := contentID(input.Target.ContentID(), relation, authority.valueTable.content, authority.seedTable.relation, authority.seedTable.endpointRelation)
	if !content.Available() {
		return nil, errors.New("link/boundary: unavailable content identity")
	}
	authority.content = content
	component := &Component{authority: authority}
	authority.component = component
	if authority.semanticReceipt, err = component.buildSemanticSourceReceipt(); err != nil {
		return nil, err
	}
	return &Draft{state: &draftState{authority: authority}}, nil
}

const moduleRelationVersion = 1

// moduleRelationID is the exact Boundary projection consumed by Module: the
// immutable Program-value relation plus the presence and ordinal of scoped
// require authority. Endpoint and bootstrap geometry are intentionally not a
// module dependency.
func moduleRelationID(valueRelation keyspace.ContentID, contract *target.Contract, require target.Operation) (id keyspace.ContentID) {
	if !valueRelation.Available() {
		return id
	}
	var requireID keyspace.ContentID
	if require != 0 {
		var ok bool
		requireID, ok = contract.OperationContentID(require)
		if !ok || !requireID.Available() {
			return id
		}
	}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/boundary/module", moduleRelationVersion) != nil || writer.Record(1) != nil || writer.Bytes(valueRelation[:]) != nil || writer.Bool(require != 0) != nil {
		return id
	}
	if require != 0 && writer.Bytes(requireID[:]) != nil || writer.Finish() != nil {
		return id
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

// Finalize consumes the construction capability and publishes an immutable
// component. All copies of the Draft share the same consumption fence.
func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return nil, errors.New("link/boundary: invalid finalization")
	}
	d.state.consumed = true
	authority := d.state.authority
	d.state.authority = nil
	component := &Component{authority: authority}
	authorityComponent := component
	// Values are issued with the final component pointer, not with a mutable
	// Draft or the enclosing Link. This keeps foreign same-ordinal handles
	// unforgeable across equivalent Boundary reseals.
	authority.component = authorityComponent
	return component, nil
}

func scopedRequireOperation(contract *target.Contract) (target.Operation, error) {
	var require target.Operation
	for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
		op, ok := contract.OperationAt(operationIndex)
		if !ok || op == 0 {
			return 0, errors.New("link/boundary: malformed Target operation")
		}
		for bindingIndex := 0; bindingIndex < contract.BindingCount(op); bindingIndex++ {
			namespace, ok := contract.BindingNamespaceAt(op, bindingIndex)
			if !ok {
				return 0, errors.New("link/boundary: malformed Target binding")
			}
			if namespace != target.BindingBuiltin {
				continue
			}
			memberCount := contract.BindingMemberCountAt(op, bindingIndex)
			if memberCount == 0 {
				return 0, errors.New("link/boundary: malformed Target binding member")
			}
			first, ok := contract.BindingMemberAt(op, bindingIndex, 0)
			if !ok {
				return 0, errors.New("link/boundary: malformed Target binding member")
			}
			isRequire, err := classifyRequireBinding(namespace, contract.BindingOwnerCountAt(op, bindingIndex), memberCount, first)
			if err != nil {
				return 0, err
			}
			if !isRequire {
				continue
			}
			if contract.BindingCount(op) != 1 {
				return 0, errors.New("link/boundary: scoped require operation has other target ingress")
			}
			if require != 0 && require != op {
				return 0, errors.New("link/boundary: conflicting scoped require operations")
			}
			require = op
		}
	}
	return require, nil
}

// classifyRequireBinding classifies the Target binding shape before any
// owner shortcut. Target itself normally rejects builtin owners, but keeping
// this check here makes Boundary fail closed if that ABI ever admits one.
func classifyRequireBinding(namespace target.BindingNamespace, ownerCount, memberCount int, firstMember string) (bool, error) {
	if namespace != target.BindingBuiltin {
		return false, nil
	}
	if memberCount == 0 {
		return false, errors.New("link/boundary: malformed Target binding member")
	}
	if firstMember != "require" {
		return false, nil
	}
	if ownerCount != 0 {
		return false, errors.New("link/boundary: builtin require binding has owner")
	}
	if memberCount != 1 {
		return false, errors.New("link/boundary: require has no member source ingress")
	}
	return true, nil
}

func contentID(targetID, applicationRelation, valueRelation, seedRelation, endpointRelation keyspace.ContentID) (id keyspace.ContentID) {
	if !targetID.Available() || !applicationRelation.Available() || !valueRelation.Available() || !seedRelation.Available() || !endpointRelation.Available() {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var writer canonical.Writer
	if writer.Reset(h, "program/link/boundary", contentVersion) != nil ||
		writer.Record(1) != nil || writer.Bytes(targetID[:]) != nil ||
		writer.Bytes(applicationRelation[:]) != nil || writer.Bytes(valueRelation[:]) != nil || writer.Bytes(seedRelation[:]) != nil || writer.Bytes(endpointRelation[:]) != nil || writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	sum := h.Sum(id[:0])
	if len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}
