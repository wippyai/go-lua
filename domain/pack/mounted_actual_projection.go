package pack

import "github.com/wippyai/go-lua/analysis/identity"

// MountedActualProjection is Pack's detached projection of one authored call
// actuals row.  The fixed sources are in the Program declaration order used
// by callRow: a method receiver first, followed by its arguments.  A
// variadic/open actuals tail is retained only as a direct link to Pack's
// canonical tail row.
//
// Endpoint and callRow remain private.  In particular, this descriptor does
// not expose a raw endpoint, Program row, Link, or an inferred actual.
type MountedActualProjection struct {
	schema *schema
	index  uint32
	sealed bool
}

// available is the hot handle check. Construction validates every fixed
// source and the optional tail before setting sealed; immutable projections
// must not replay that O(n) proof on every accessor.
func (projection MountedActualProjection) available() bool {
	return projection.sealed && projection.schema != nil && projection.schema.owner != nil &&
		projection.schema.owner.valid() && uint64(projection.index) < uint64(len(projection.schema.calls))
}

// valid is construction-only full validation.
func (projection MountedActualProjection) valid() bool {
	if !projection.available() {
		return false
	}
	row := projection.schema.calls[projection.index]
	if !projection.schema.validMountedCall(projection.index, row) {
		return false
	}
	for _, endpoint := range row.fixed {
		source, sourceOK := projection.sourceFor(endpoint)
		if !sourceOK || source.Module() != row.moduleKey {
			return false
		}
	}
	if !row.hasTail {
		return row.tailRoot == 0
	}
	_, portOK := projection.schema.tailPort(row.tailRoot)
	if !portOK {
		return false
	}
	root := projection.schema.roots[row.tailRoot]
	tail := projection.schema.tails[root.sourceIndex]
	return tail.moduleKey == row.moduleKey && tail.root == row.tailRoot && tail.valueID.Available()
}

func (projection MountedActualProjection) sourceFor(endpoint Endpoint) (SemanticSource, bool) {
	if projection.schema == nil {
		return SemanticSource{}, false
	}
	return projection.schema.sourceForEndpoint(endpoint)
}

// Valid reports whether this descriptor was issued by a sealed Pack Schema.
// Full row validation occurs once before issuance.
func (projection MountedActualProjection) Valid() bool { return projection.available() }

// OwnedBy proves that the descriptor was issued by this exact Pack Schema.
// Equivalent independently sealed schemas intentionally do not pass.
func (projection MountedActualProjection) OwnedBy(schema *Schema) bool {
	return schema != nil && schema.state != nil && projection.available() && projection.schema == schema.state
}

// ActualCount reports the number of fixed actual sources.  A receiver, when
// present, is included at index zero.
func (projection MountedActualProjection) ActualCount() int {
	if !projection.available() {
		return 0
	}
	return len(projection.schema.calls[projection.index].fixed)
}

// ActualAt returns one fixed actual source in receiver-then-argument order.
func (projection MountedActualProjection) ActualAt(index int) (SemanticSource, bool) {
	if !projection.available() {
		return SemanticSource{}, false
	}
	row := projection.schema.calls[projection.index]
	if index < 0 || index >= len(row.fixed) {
		return SemanticSource{}, false
	}
	source, ok := projection.sourceFor(row.fixed[index])
	return source, ok && source.Module() == row.moduleKey
}

// TailID returns the exact mounted semantic identity of the actuals tail.
func (projection MountedActualProjection) TailID() (identity.ContentID, bool) {
	if !projection.available() {
		return identity.ContentID{}, false
	}
	row := projection.schema.calls[projection.index]
	if !row.hasTail {
		return identity.ContentID{}, false
	}
	root := projection.schema.roots[row.tailRoot]
	return projection.schema.tails[root.sourceIndex].valueID, true
}

// MountedActualProjection projects one sealed mounted call in O(1) without
// copying its fixed actual row. The descriptor retains the schema's immutable
// sealed Endpoint image and resolves sources lazily in O(1); callers never
// receive callRow or Endpoint.
func (schema *Schema) MountedActualProjection(module, callID identity.ContentID) (MountedActualProjection, bool) {
	if schema == nil || schema.state == nil || schema.state.owner == nil || !schema.state.owner.valid() || !schema.state.linkOwner.Available() || !module.Available() || !callID.Available() {
		return MountedActualProjection{}, false
	}
	index, found := schema.state.artifactCalls[artifactCallKey{module, callID}]
	if !found || uint64(index) >= uint64(len(schema.state.calls)) {
		return MountedActualProjection{}, false
	}
	row := schema.state.calls[index]
	if !schema.state.validMountedCall(index, row) || row.moduleKey != module || row.occurrenceID != callID ||
		schema.state.artifactCalls[artifactCallKey{row.moduleKey, row.occurrenceID}] != index {
		return MountedActualProjection{}, false
	}

	projection := MountedActualProjection{schema: schema.state, index: index}
	projection.sealed = true
	return projection, projection.valid()
}
