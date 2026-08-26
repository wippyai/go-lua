package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const planDigestDomain = "analysis/relation/mount/arrangement/plan/v1"

// Derive resolves every logical access required by a checked certificate and
// binds the immutable dependent evidence catalog supplied by the mount
// snapshot. Access handles and dependent evidence have separate owners;
// neither is rediscovered through a runtime type assertion or callback.
func Derive(cert certificate.Certificate, book address.Book, inventory Inventory, evidence expand.Catalog, partitionDirectories []binding.PartitionDirectory) (Plan, bool) {
	if inventory == nil || !cert.Available() || !book.Available() || partitionDirectories == nil {
		return Plan{}, false
	}
	partitions := cert.CorrelationPartitions()
	partitionDirectories = append([]binding.PartitionDirectory{}, partitionDirectories...)
	fence := inventory.Fence()
	if !fence.Available() || !fence.Same(book.Fence()) || fence.SchemaID() != cert.SchemaID() || fence.CertificateDigest() != cert.Digest() {
		return Plan{}, false
	}
	if !validatePartitionDirectories(fence, partitions, partitionDirectories) {
		return Plan{}, false
	}
	state := deriveState{
		book:           book,
		relations:      make(map[model.RelationID]model.RelationSchema),
		keys:           make(map[model.KeyID]model.KeySchema),
		columns:        make(map[model.ColumnID]model.ColumnSchema),
		scopeDefs:      make(map[model.ScopeID]model.ScopeSchema),
		signatures:     make(map[signature.Identity]signature.Signature),
		inventory:      inventory,
		expandEvidence: evidence,
		usedExpand:     make(map[identity.ContentID]struct{}),
		proposalMerges: make(map[identity.ContentID]bool),
	}
	contributions := cert.Contributions()
	for index, contribution := range contributions {
		if !contribution.Available() {
			return Plan{}, false
		}
		for _, prior := range contributions[:index] {
			if prior.Port() == contribution.Port() {
				return Plan{}, false
			}
		}
	}
	relations := cert.Relations()
	for _, value := range relations {
		if !value.Available() || !value.ID().Available() {
			return Plan{}, false
		}
		if _, exists := state.relations[value.ID()]; exists {
			return Plan{}, false
		}
		state.relations[value.ID()] = value
	}
	for _, value := range cert.Keys() {
		if !value.Available() || !value.ID().Available() || value.ID().Relation() != value.Relation() {
			return Plan{}, false
		}
		if _, exists := state.keys[value.ID()]; exists {
			return Plan{}, false
		}
		state.keys[value.ID()] = value
	}
	for _, value := range cert.Columns() {
		if !value.Available() || !value.ID().Available() {
			return Plan{}, false
		}
		if _, exists := state.columns[value.ID()]; exists {
			return Plan{}, false
		}
		state.columns[value.ID()] = value
	}
	for _, value := range cert.Scopes() {
		if !value.Available() || !value.ID().Available() {
			return Plan{}, false
		}
		if _, exists := state.scopeDefs[value.ID()]; exists {
			return Plan{}, false
		}
		state.scopeDefs[value.ID()] = value
	}
	for _, value := range cert.Signatures() {
		if !value.Available() || !value.Identity().Available() {
			return Plan{}, false
		}
		if _, exists := state.signatures[value.Identity()]; exists {
			return Plan{}, false
		}
		state.signatures[value.Identity()] = value
	}

	// Every certified semantic signature is a mount obligation, even if a
	// future planner drops an unreachable expression.  This keeps semantic
	// delivery and denominator admission complete for the whole census.
	signatureValues := make([]signature.Signature, 0, len(state.signatures))
	for _, value := range state.signatures {
		signatureValues = append(signatureValues, value)
	}
	sort.SliceStable(signatureValues, func(left, right int) bool {
		return identityLess(signatureValues[left].Digest(), signatureValues[right].Digest())
	})
	for _, value := range signatureValues {
		if !state.addSignature(value) {
			return Plan{}, false
		}
	}
	expressions := cert.Expressions()
	for _, value := range expressions {
		if !value.Available() || !value.ID().Available() || value.Expression() == nil {
			return Plan{}, false
		}
		if !bookExpression(book, value.ID()) {
			return Plan{}, false
		}
	}
	for _, value := range expressions {
		if !state.deriveExpression(value.Expression()) {
			return Plan{}, false
		}
	}
	for _, expressionDigest := range evidence.Expressions() {
		if _, used := state.usedExpand[expressionDigest]; !used {
			return Plan{}, false
		}
	}
	for _, dependency := range cert.Dependencies() {
		if !dependency.Available() || !dependency.ID().Available() || !bookDependency(book, dependency.ID()) {
			return Plan{}, false
		}
		for _, relation := range append(dependency.Reads(), dependency.Writes()...) {
			if !relation.Available() || !state.addRelationAccess(relation.ID()) {
				return Plan{}, false
			}
		}
	}

	state.accesses = canonicalizeAccesses(state.accesses)
	state.deliveries = canonicalizeDeliveries(state.deliveries)
	for _, access := range state.accesses {
		if !bookAccess(book, access) {
			return Plan{}, false
		}
	}
	// An Access is the logical delivered row vector.  A single vector can
	// legitimately need more than one physical coordinate (for example the
	// authored Input vector and one or more owner-issued correlation indexes).
	// Resolve the inventory handle once per logical Access, then issue one
	// immutable Layout for every sealed coordinate variant.  Refusing the
	// second variant here would make a valid Merge/Apply composition
	// impossible; selecting the first variant would make runtime redemption
	// depend on declaration order.
	layouts := make([]Layout, 0, len(state.accesses)+len(state.correspondence))
	seenHandles := make(map[Handle]Access, len(state.accesses))
	for _, access := range state.accesses {
		handle, ok := inventory.Resolve(access)
		if !ok || !handle.Available() || !handle.ValidFor(fence) {
			return Plan{}, false
		}
		if prior, duplicate := seenHandles[handle]; duplicate && !prior.Equal(access) {
			return Plan{}, false
		}
		seenHandles[handle] = cloneAccess(access)
		specs := []physicalVector{{access: access, coordinateClass: CoordinateClassNone}}
		if access.Key().Available() {
			key, keyOK := state.keys[access.Key()]
			if !keyOK || !key.Available() {
				return Plan{}, false
			}
			specs[0].keyColumns = key.Columns()
			specs[0].coordinateClass = CoordinateClassDeclaredKey
		} else {
			// Keep the ordinary row-vector realization even when it also has
			// correspondence coordinates.  Input/scan and population readers
			// redeem this neutral layout; Join, Merge, Apply, and Expand select
			// their exact class below during execution lowering.
			specs = append(specs, state.physicalCoordinates(access)...)
		}
		for _, spec := range specs {
			layout, layoutOK := newLayoutWithClass(fence, handle, access, spec.keyColumns, spec.coordinateClass)
			if !layoutOK {
				return Plan{}, false
			}
			layouts = append(layouts, layout)
		}
	}

	// Mount consumes the certificate's sealed recurrence projection.  It must
	// not reopen plan declarations or the checker proof after admission; the
	// projection is the only runtime scheduling input.
	execution, executionOK := deriveExecution(fence, book, expressions, cert.Recurrence(), relations, layouts, state.deliveries, signatureValues, state.expandEvidence, partitionDirectories, partitions, state.proposalMerges)
	if !executionOK || !execution.Available() {
		return Plan{}, false
	}

	contributionDirectory, contributionsOK := newContributionDirectory(fence, contributions)
	if !contributionsOK {
		return Plan{}, false
	}
	data := &planData{
		fence:         fence,
		layouts:       layouts,
		deliveries:    state.deliveries,
		contributions: contributionDirectory,
		execution:     execution,
	}
	logicalParts := make([][]byte, 0, len(data.layouts)+len(data.deliveries))
	appendPlanDigestParts(&logicalParts, *data)
	logicalParts = append(logicalParts, contentBytes(execution.LogicalDigest()))
	logicalDigest, ok := identity.DeriveContentID(planDigestDomain+"/logical", logicalParts...)
	if !ok {
		return Plan{}, false
	}
	physicalParts := append([][]byte(nil), logicalParts...)
	for _, layout := range data.layouts {
		physicalParts = append(physicalParts, contentBytes(layout.Digest()))
	}
	physicalParts = append(physicalParts, contentBytes(execution.Digest()))
	digest, ok := identity.DeriveContentID(planDigestDomain, physicalParts...)
	if !ok {
		return Plan{}, false
	}
	data.logicalDigest = logicalDigest
	data.digest = digest
	return Plan{data: data}, true
}

type deriveState struct {
	book           address.Book
	relations      map[model.RelationID]model.RelationSchema
	keys           map[model.KeyID]model.KeySchema
	columns        map[model.ColumnID]model.ColumnSchema
	scopeDefs      map[model.ScopeID]model.ScopeSchema
	signatures     map[signature.Identity]signature.Signature
	inventory      Inventory
	expandEvidence expand.Catalog
	accesses       []Access
	// correspondence contains the exact ordered vectors used by checked Join
	// nodes and Project target-key mappings. It is a cold mount fact, not a
	// second Access vocabulary: the canonical Access remains the key in
	// state.accesses while this side list tells layout construction to promote
	// the same Access into an indexed tuple coordinate.
	correspondence []physicalVector
	deliveries     []DeliveryRequirement
	visiting       map[identity.ContentID]bool
	visited        map[identity.ContentID]bool
	usedExpand     map[identity.ContentID]struct{}
	// proposalMerges is the one mount-time disposition table for Merge.  It is
	// derived while logical accesses are collected and then redeemed by
	// physical lowering; execution never independently reclassifies syntax.
	proposalMerges map[identity.ContentID]bool
}

type physicalVector struct {
	access          Access
	keyColumns      []model.ColumnID
	coordinateClass CoordinateClass
}

// relationColumns is the cold declaration lookup used only while deriving
// full-row publication vectors. Runtime receives the resulting sealed Layout
// and never reopens this relation catalogue.
func (state *deriveState) relationColumns(relation model.RelationID) ([]model.ColumnID, bool) {
	if state == nil || !relation.Available() {
		return nil, false
	}
	definition, ok := state.relations[relation]
	if !ok || !definition.Available() {
		return nil, false
	}
	return definition.Columns(), true
}

func (state *deriveState) addSignature(value signature.Signature) bool {
	for index, input := range value.Inputs() {
		requirement, ok := newDeliveryRequirement(value.Identity(), index, input)
		if !ok || !state.addInputAccess(input) {
			return false
		}
		state.deliveries = append(state.deliveries, requirement)
	}
	outputRelations := make([]model.RelationID, 0)
	seenOutputRelations := make(map[model.RelationID]struct{})
	for _, output := range value.Outputs() {
		if !output.Available() {
			return false
		}
		if !output.Denominator.Available() || !state.addKeyAccess(output.Denominator.Key()) {
			return false
		}
		access, ok := NewVectorAccess(output.Relation, []model.ColumnID{output.Column})
		if !ok || !state.addAccess(access) {
			return false
		}
		if _, seen := seenOutputRelations[output.Relation]; !seen {
			seenOutputRelations[output.Relation] = struct{}{}
			outputRelations = append(outputRelations, output.Relation)
		}
	}
	// An output declaration publishes rows, not isolated cells. Seal the
	// owner's complete authored row vector alongside the per-column indexes so
	// downstream readers can redeem the row without reconstructing an Access
	// from schema metadata. The relation schema is the sole vector authority;
	// signature output order is not restated here.
	for _, relation := range outputRelations {
		definition, ok := state.relations[relation]
		if !ok || !definition.Available() {
			return false
		}
		access, ok := NewVectorAccess(relation, definition.Columns())
		if !ok || !state.addAccess(access) {
			return false
		}
	}
	return true
}

func (state *deriveState) addInputAccess(input signature.Input) bool {
	access, ok := deliveryAccess(input)
	if !ok || !state.addAccess(access) {
		return false
	}
	if input.Delivery.IsSpan() {
		order, orderOK := NewKeyAccess(input.Delivery.OrderKey())
		if !orderOK || !state.addAccess(order) {
			return false
		}
	}
	return true
}

func (state *deriveState) deriveExpression(expression algebra.Expression) bool {
	if expression == nil || !expression.Digest().Available() {
		return false
	}
	if state.visiting == nil {
		state.visiting = make(map[identity.ContentID]bool)
		state.visited = make(map[identity.ContentID]bool)
	}
	digest := expression.Digest()
	if state.visiting[digest] {
		return false
	}
	if state.visited[digest] {
		return true
	}
	state.visiting[digest] = true
	defer delete(state.visiting, digest)
	var ok bool
	switch value := expression.(type) {
	case algebra.Input:
		ok = state.addInputExpression(value)
	case *algebra.Input:
		ok = value != nil && state.addInputExpression(*value)
	case algebra.Select:
		ok = state.deriveExpression(value.Child())
		if ok {
			ok = state.addScope(value.Contract().Scope())
		}
		if ok {
			ok = state.addScopeVector(value.Contract().Scope())
		}
	case *algebra.Select:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				ok = state.addScope(value.Contract().Scope()) && state.addScopeVector(value.Contract().Scope())
			}
		}
	case algebra.Project:
		ok = state.deriveExpression(value.Child())
		if ok {
			contract := value.Contract()
			// Project reads complete destination rows so its sealed target tuple
			// can retain every mapped/key cell. The Key access remains a separate
			// equality/index authority; it is not itself a row projection.
			ok = state.addInputRelationAccess(contract.Target()) && state.addKeyAccess(contract.Key())
			if ok {
				mappings := contract.Mappings()
				ok = state.addMappingVectors(mappings) && state.addProjectCorrespondence(mappings, contract.Key())
			}
		}
	case algebra.ColumnProject:
		ok = state.deriveExpression(value.Child())
		if ok {
			slots := value.Contract().Slots()
			columns := make([]model.ColumnID, len(slots))
			for index, slot := range slots {
				columns[index] = slot.Column()
			}
			ok = len(columns) != 0 && state.addVectorAccess(columns)
		}
	case *algebra.ColumnProject:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				slots := value.Contract().Slots()
				columns := make([]model.ColumnID, len(slots))
				for index, slot := range slots {
					columns[index] = slot.Column()
				}
				ok = len(columns) != 0 && state.addVectorAccess(columns)
			}
		}
	case algebra.Expand:
		ok = state.deriveExpression(value.Child())
		if ok {
			ok = state.addExpand(value)
		}
	case *algebra.Expand:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				ok = state.addExpand(*value)
			}
		}
	case *algebra.Project:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				contract := value.Contract()
				mappings := contract.Mappings()
				ok = state.addInputRelationAccess(contract.Target()) && state.addKeyAccess(contract.Key()) && state.addMappingVectors(mappings) && state.addProjectCorrespondence(mappings, contract.Key())
			}
		}
	case algebra.Join:
		ok = state.deriveExpression(value.Left())
		if ok {
			ok = state.deriveExpression(value.Right())
		}
		if ok {
			contract := value.Contract()
			ok = state.addCorrespondenceAccess(contract.LeftColumns()) && state.addCorrespondenceAccess(contract.RightColumns())
		}
	case *algebra.Join:
		if value != nil {
			ok = state.deriveExpression(value.Left())
			if ok {
				ok = state.deriveExpression(value.Right())
			}
			if ok {
				contract := value.Contract()
				ok = state.addCorrespondenceAccess(contract.LeftColumns()) && state.addCorrespondenceAccess(contract.RightColumns())
			}
		}
	case algebra.Merge:
		ok = true
		inputs := value.Inputs()
		proposalMerge := state.sealMergeDisposition(value.Digest(), inputs)
		for _, child := range inputs {
			ok = ok && state.deriveExpression(child)
		}
		if ok {
			key := value.Contract().Key()
			ok = state.addKeyAccess(key)
			if ok {
				keySchema, keyOK := state.keys[key]
				ok = keyOK && keySchema.Available()
				if ok {
					for _, child := range inputs {
						columns, columnsOK := state.expressionColumns(child)
						if !columnsOK {
							ok = false
							break
						}
						if proposalMerge {
							// The application result is the one keyed destination
							// authority. A carried projection may omit payload cells
							// from its semantic shape, but the destination row still
							// needs one mutable lookup coordinate so every Merge fact
							// is indexed by the owner-issued key.
							destinationColumns, destinationOK := state.relationColumns(key.Relation())
							ok = destinationOK && state.addMergeCorrespondence(destinationColumns, keySchema.Columns())
						} else {
							ok = state.addMergeCorrespondence(columns, keySchema.Columns())
						}
						if !ok {
							break
						}
					}
				}
			}
		}
	case *algebra.Merge:
		if value != nil {
			ok = true
			inputs := value.Inputs()
			proposalMerge := state.sealMergeDisposition(value.Digest(), inputs)
			for _, child := range inputs {
				ok = ok && state.deriveExpression(child)
			}
			if ok {
				key := value.Contract().Key()
				ok = state.addKeyAccess(key)
				if ok {
					keySchema, keyOK := state.keys[key]
					ok = keyOK && keySchema.Available()
					if ok {
						for _, child := range inputs {
							columns, columnsOK := state.expressionColumns(child)
							if !columnsOK {
								ok = false
								break
							}
							if proposalMerge {
								// The application result is the one keyed destination
								// authority. A carried projection may omit payload cells
								// from its semantic shape, but the destination row still
								// needs one mutable lookup coordinate so every Merge fact
								// is indexed by the owner-issued key.
								destinationColumns, destinationOK := state.relationColumns(key.Relation())
								ok = destinationOK && state.addMergeCorrespondence(destinationColumns, keySchema.Columns())
							} else {
								ok = state.addMergeCorrespondence(columns, keySchema.Columns())
							}
							if !ok {
								break
							}
						}
					}
				}
			}
		}
	case algebra.Group:
		ok = state.deriveExpression(value.Child())
		if ok {
			ok = state.addKeyAccess(value.Contract().Key())
		}
	case *algebra.Group:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				ok = state.addKeyAccess(value.Contract().Key())
			}
		}
	case algebra.Complete:
		ok = state.deriveExpression(value.Child())
		if ok {
			denominator := value.Denominator()
			ok = denominator.Available() && state.addKeyAccess(denominator.Key())
		}
	case *algebra.Complete:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				denominator := value.Denominator()
				ok = denominator.Available() && state.addKeyAccess(denominator.Key())
			}
		}
	case algebra.Apply:
		ok = true
		for _, child := range value.Inputs() {
			ok = ok && state.deriveExpression(child)
		}
		if ok {
			_, ok = state.signatures[value.Contract().Operation()]
			if ok {
				ok = state.addApplyCorrelation(value.Contract().Correlation())
			}
		}
	case *algebra.Apply:
		if value != nil {
			ok = true
			for _, child := range value.Inputs() {
				ok = ok && state.deriveExpression(child)
			}
			if ok {
				_, ok = state.signatures[value.Contract().Operation()]
				if ok {
					ok = state.addApplyCorrelation(value.Contract().Correlation())
				}
			}
		}
	case algebra.Publish:
		ok = state.deriveExpression(value.Child())
		if ok {
			contract := value.Contract()
			columns := contract.Columns()
			if len(columns) == 0 {
				var columnsOK bool
				columns, columnsOK = state.relationColumns(contract.Destination())
				ok = columnsOK
			}
			if ok {
				ok = len(columns) != 0 && state.addRelationAccess(contract.Destination()) && state.addKeyAccess(contract.Key()) && state.addVectorAccess(columns)
			}
		}
	case *algebra.Publish:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				contract := value.Contract()
				columns := contract.Columns()
				if len(columns) == 0 {
					var columnsOK bool
					columns, columnsOK = state.relationColumns(contract.Destination())
					ok = columnsOK
				}
				if ok {
					ok = len(columns) != 0 && state.addRelationAccess(contract.Destination()) && state.addKeyAccess(contract.Key()) && state.addVectorAccess(columns)
				}
			}
		}
	default:
		ok = false
	}
	if ok {
		state.visited[digest] = true
	}
	return ok
}

// addApplyCorrelation retains only the independent population driver. Exact
// child extent selection happens later, after the mounted tree gives every
// Input and Complete occurrence a root-relative path.
func (state *deriveState) addApplyCorrelation(correlation algebra.ApplyCorrelation) bool {
	if state == nil {
		return false
	}
	if !correlation.Specified() {
		return true
	}
	if !correlation.Available() {
		return false
	}
	// Population is the independent closed Q authority. It is not inferred
	// from a child. Mount retains only the declared coordinate projection:
	// unrelated payload or derived columns must never decide whether an
	// owner-issued population row exists. The denominator key remains a
	// logical witness only, never a fabricated RowID-to-key inverse.
	population := correlation.Population()
	if !population.Available() || population.Relation() != correlation.Coordinate().Relation() {
		return false
	}
	populationSchema, populationOK := state.relations[population.Relation()]
	if !populationOK || !populationSchema.Available() {
		return false
	}
	populationColumns := populationSchema.Columns()
	if len(populationColumns) == 0 || !containsColumn(populationColumns, correlation.Coordinate()) {
		return false
	}
	coordinateSchema, coordinateOK := state.columns[correlation.Coordinate()]
	if !coordinateOK || !coordinateSchema.Available() || coordinateSchema.Type() != correlation.Type() {
		return false
	}
	driverAccess, driverAccessOK := NewVectorAccess(population.Relation(), []model.ColumnID{correlation.Coordinate()})
	if !driverAccessOK || !state.addAccess(driverAccess) {
		return false
	}
	return true
}

func (state *deriveState) addRelationAccess(relation model.RelationID) bool {
	if !relation.Available() {
		return false
	}
	definition, ok := state.relations[relation]
	if !ok {
		return false
	}
	access, ok := NewRelationAccess(relation)
	return ok && state.addScope(definition.Scope()) && state.addAccess(access)
}

// addInputRelationAccess declares the complete row used by structural operators
// such as Project and Expand. Input leaves do not use this helper: their sealed
// algebra.Input carries the occurrence-local projection and is added by
// addInputExpression below.
func (state *deriveState) addInputRelationAccess(relation model.RelationID) bool {
	if !state.addRelationAccess(relation) {
		return false
	}
	definition, ok := state.relations[relation]
	if !ok || !definition.Available() {
		return false
	}
	access, ok := NewVectorAccess(relation, definition.Columns())
	return ok && state.addAccess(access)
}

// addInputExpression adds the exact vector carried by one sealed Input
// occurrence. No relation-level union is permitted: two Inputs over the same
// relation remain distinct by their algebra digest and may expose different
// columns. AllColumns is an explicit source contract for direct authors and is
// expanded only here, at the mount boundary.
func (state *deriveState) addInputExpression(input algebra.Input) bool {
	if state == nil || !input.Available() || !state.addRelationAccess(input.Relation()) {
		return false
	}
	columns := input.Columns()
	if input.IsAllColumns() {
		var ok bool
		columns, ok = state.relationColumns(input.Relation())
		if !ok {
			return false
		}
	}
	definition, relationOK := state.relations[input.Relation()]
	if !relationOK || !definition.Available() {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		columnDefinition, columnOK := state.columns[column]
		if !column.Available() || column.Relation() != input.Relation() || !columnOK || !columnDefinition.Available() || columnDefinition.ID() != column || !definition.HasColumn(column) {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	access, accessOK := NewVectorAccess(input.Relation(), columns)
	return accessOK && state.addAccess(access)
}

func (state *deriveState) addKeyAccess(key model.KeyID) bool {
	if !key.Available() {
		return false
	}
	if _, ok := state.keys[key]; !ok {
		return false
	}
	access, ok := NewKeyAccess(key)
	return ok && state.addAccess(access)
}

func (state *deriveState) addVectorAccess(columns []model.ColumnID) bool {
	if len(columns) == 0 {
		return false
	}
	access, ok := NewVectorAccess(columns[0].Relation(), columns)
	return ok && state.addAccess(access)
}

// addExpand validates the immutable owner evidence supplied by the mount
// snapshot. No owner callback, ordinal, or token issuer is reachable from
// arrangement after this point.
func (state *deriveState) addExpand(value algebra.Expand) bool {
	if state == nil || !value.Contract().Available() || !state.expandEvidence.Available() {
		return false
	}
	contract := value.Contract()
	evidence, evidenceOK := state.expandEvidence.At(value.Digest())
	if !evidenceOK {
		return false
	}
	if !state.addInputRelationAccess(contract.Reader()) {
		return false
	}
	// Reader.Lookup must redeem complete R rows through the exact declared
	// key while still delivering the full authored R vector. Promote that
	// already-added vector into a mount-sealed lookup coordinate; runtime does
	// not construct a second key layout or scan the reader relation.
	readerColumns, readerColumnsOK := state.relationColumns(contract.Reader())
	readerAccess, readerAccessOK := NewVectorAccess(contract.Reader(), readerColumns)
	if !readerColumnsOK || !readerAccessOK || !state.addPhysicalVector(readerAccess, []model.ColumnID{contract.Key()}, CoordinateClassLookupOnly) {
		return false
	}
	key, keyOK := state.keyForColumn(contract.Key())
	if !keyOK || !state.addKeyAccess(key) {
		return false
	}
	keySchema, schemaOK := state.keys[key]
	if !schemaOK || !keySchema.Available() {
		return false
	}
	keyColumns := keySchema.Columns()
	if len(keyColumns) != 1 || keyColumns[0] != contract.Key() {
		return false
	}
	keyColumn, columnOK := state.columns[contract.Key()]
	if !columnOK || !keyColumn.Available() {
		return false
	}
	addressFence := state.inventory.Fence()
	runtimeFence := evidence.Fence()
	if !runtimeFence.Available() || runtimeFence.Schema() != addressFence.SchemaID() || runtimeFence.Mount() != addressFence.MountID() || runtimeFence.Generation() != addressFence.Generation() || evidence.Contract() != contract || evidence.KeyType() != keyColumn.Type() {
		return false
	}
	state.usedExpand[value.Digest()] = struct{}{}
	return true
}

func (state *deriveState) keyForColumn(column model.ColumnID) (model.KeyID, bool) {
	if state == nil || !column.Available() {
		return model.KeyID{}, false
	}
	var result model.KeyID
	for keyID, definition := range state.keys {
		if !definition.Available() || definition.Relation() != column.Relation() {
			continue
		}
		for _, candidate := range definition.Columns() {
			if candidate != column {
				continue
			}
			if result.Available() {
				return model.KeyID{}, false
			}
			result = keyID
		}
	}
	return result, result.Available()
}

// addCorrespondenceAccess records one exact ordered Join vector in the same
// logical Access census as every other arrangement requirement.  The side
// list only affects physical layout construction; it does not introduce a
// second access/index type or duplicate resolver call.
func (state *deriveState) addCorrespondenceAccess(columns []model.ColumnID) bool {
	if len(columns) == 0 {
		return false
	}
	access, ok := NewVectorAccess(columns[0].Relation(), columns)
	if !ok || !state.addAccess(access) {
		return false
	}
	return state.addPhysicalVector(access, columns, CoordinateClassStableCorrespondence)
}

func (state *deriveState) addPhysicalVector(access Access, keyColumns []model.ColumnID, coordinateClass CoordinateClass) bool {
	if state == nil || !access.Available() || len(keyColumns) == 0 {
		return false
	}
	if coordinateClass != CoordinateClassStableCorrespondence && coordinateClass != CoordinateClassLookupOnly {
		return false
	}
	if access.Relation() != keyColumns[0].Relation() {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(keyColumns))
	for _, column := range keyColumns {
		if !column.Available() || column.Relation() != access.Relation() {
			return false
		}
		if _, duplicate := seen[column]; duplicate {
			return false
		}
		seen[column] = struct{}{}
	}
	for index := range state.correspondence {
		candidate := &state.correspondence[index]
		if !candidate.access.Equal(access) {
			continue
		}
		if !sameColumns(candidate.keyColumns, keyColumns) {
			continue
		}
		// A physical Access/vector cannot carry two ownership proofs. Refuse
		// a mixed direct-correspondence and Merge use rather than silently
		// upgrading a mutable fact to the immutable stable fence.
		if candidate.coordinateClass != coordinateClass {
			return false
		}
		return true
	}
	state.correspondence = append(state.correspondence, physicalVector{access: cloneAccess(access), keyColumns: append([]model.ColumnID(nil), keyColumns...), coordinateClass: coordinateClass})
	return true
}

func (state *deriveState) physicalCoordinates(access Access) []physicalVector {
	if state == nil || !access.Available() {
		return nil
	}
	result := make([]physicalVector, 0)
	for _, candidate := range state.correspondence {
		if !candidate.access.Equal(access) {
			continue
		}
		duplicate := false
		for _, prior := range result {
			if prior.coordinateClass == candidate.coordinateClass && sameColumns(prior.keyColumns, candidate.keyColumns) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		result = append(result, physicalVector{
			access:          cloneAccess(candidate.access),
			keyColumns:      append([]model.ColumnID(nil), candidate.keyColumns...),
			coordinateClass: candidate.coordinateClass,
		})
	}
	return result
}

// physicalCoordinate is retained only for the package-local law that checks
// the single-variant case. Runtime and Derive use physicalCoordinates; a
// logical Access with more than one physical realization is deliberately
// ambiguous here and refuses rather than selecting the first entry.
func (state *deriveState) physicalCoordinate(access Access) ([]model.ColumnID, CoordinateClass, bool) {
	variants := state.physicalCoordinates(access)
	if len(variants) != 1 {
		return nil, CoordinateClassInvalid, false
	}
	return append([]model.ColumnID(nil), variants[0].keyColumns...), variants[0].coordinateClass, true
}

// expressionColumns is the mount-time row-shape authority used for a sealed
// physical correspondence. Runtime receives only the resulting Layout.
func (state *deriveState) expressionColumns(expression algebra.Expression) ([]model.ColumnID, bool) {
	if state == nil || expression == nil {
		return nil, false
	}
	var columns []model.ColumnID
	switch value := expression.(type) {
	case algebra.Input:
		if value.AllColumns() {
			columns, _ = state.relationColumns(value.Relation())
		} else {
			columns = value.Columns()
		}
	case *algebra.Input:
		if value != nil {
			return state.expressionColumns(algebra.Input(*value))
		}
	case algebra.Select:
		return state.expressionColumns(value.Child())
	case *algebra.Select:
		if value != nil {
			return state.expressionColumns(algebra.Select(*value))
		}
	case algebra.Project:
		columns, _ = state.relationColumns(value.Contract().Target())
	case *algebra.Project:
		if value != nil {
			return state.expressionColumns(algebra.Project(*value))
		}
	case algebra.ColumnProject:
		slots := value.Contract().Slots()
		columns = make([]model.ColumnID, len(slots))
		for index, slot := range slots {
			columns[index] = slot.Column()
		}
	case *algebra.ColumnProject:
		if value != nil {
			return state.expressionColumns(algebra.ColumnProject(*value))
		}
	case algebra.Expand:
		child, childOK := state.expressionColumns(value.Child())
		reader, readerOK := state.relationColumns(value.Contract().Reader())
		if !childOK || !readerOK {
			return nil, false
		}
		columns = append(append([]model.ColumnID(nil), child...), reader...)
	case *algebra.Expand:
		if value != nil {
			return state.expressionColumns(algebra.Expand(*value))
		}
	case algebra.Join:
		left, leftOK := state.expressionColumns(value.Left())
		right, rightOK := state.expressionColumns(value.Right())
		if !leftOK || !rightOK {
			return nil, false
		}
		columns = append(append([]model.ColumnID(nil), left...), right...)
	case *algebra.Join:
		if value != nil {
			return state.expressionColumns(algebra.Join(*value))
		}
	case algebra.Merge:
		inputs := value.Inputs()
		if len(inputs) == 0 {
			return nil, false
		}
		columns, _ = state.expressionColumns(inputs[0])
		for _, child := range inputs[1:] {
			other, otherOK := state.expressionColumns(child)
			if !otherOK || !sameColumns(columns, other) {
				return nil, false
			}
		}
	case *algebra.Merge:
		if value != nil {
			return state.expressionColumns(algebra.Merge(*value))
		}
	case algebra.Group:
		return state.expressionColumns(value.Child())
	case *algebra.Group:
		if value != nil {
			return state.expressionColumns(algebra.Group(*value))
		}
	case algebra.Complete:
		return state.expressionColumns(value.Child())
	case *algebra.Complete:
		if value != nil {
			return state.expressionColumns(algebra.Complete(*value))
		}
	case algebra.Apply:
		signatureValue, ok := state.signatures[value.Contract().Operation()]
		if !ok || !signatureValue.Available() {
			return nil, false
		}
		outputs := signatureValue.Outputs()
		columns = make([]model.ColumnID, len(outputs))
		for index, output := range outputs {
			if !output.Available() || index > 0 && output.Relation != outputs[0].Relation {
				return nil, false
			}
			columns[index] = output.Column
		}
	case *algebra.Apply:
		if value != nil {
			return state.expressionColumns(algebra.Apply(*value))
		}
	case algebra.Publish:
		columns = value.Contract().Columns()
		if len(columns) == 0 {
			columns, _ = state.relationColumns(value.Contract().Destination())
		}
	case *algebra.Publish:
		if value != nil {
			return state.expressionColumns(algebra.Publish(*value))
		}
	default:
		return nil, false
	}
	if len(columns) == 0 {
		return nil, false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() {
			return nil, false
		}
		if _, duplicate := seen[column]; duplicate {
			return nil, false
		}
		seen[column] = struct{}{}
	}
	return append([]model.ColumnID(nil), columns...), true
}

// expressionCarriesProposal mirrors the closed evaluator ABI: Apply creates
// a proposal sidecar, and only ColumnProject or another Merge may preserve it.
// It deliberately does not search through arbitrary relational operators.
func expressionCarriesProposal(expression algebra.Expression) bool {
	if expression == nil {
		return false
	}
	switch value := expression.(type) {
	case algebra.Apply:
		return true
	case *algebra.Apply:
		return value != nil
	case algebra.ColumnProject:
		return expressionCarriesProposal(value.Child())
	case *algebra.ColumnProject:
		return value != nil && expressionCarriesProposal(value.Child())
	case algebra.Merge:
		return mergeCarriesProposal(value.Inputs())
	case *algebra.Merge:
		return value != nil && mergeCarriesProposal(value.Inputs())
	}
	return false
}

func mergeCarriesProposal(inputs []algebra.Expression) bool {
	for _, child := range inputs {
		if expressionCarriesProposal(child) {
			return true
		}
	}
	return false
}

func (state *deriveState) sealMergeDisposition(digest identity.ContentID, inputs []algebra.Expression) bool {
	if state == nil || !digest.Available() || state.proposalMerges == nil {
		return false
	}
	if value, exists := state.proposalMerges[digest]; exists {
		return value
	}
	value := mergeCarriesProposal(inputs)
	state.proposalMerges[digest] = value
	return value
}

// addMergeCorrespondence promotes the authored Merge key vector to each
// alternative's delivered row layout. The logical Access remains unkeyed.
func (state *deriveState) addMergeCorrespondence(columns, keyColumns []model.ColumnID) bool {
	if state == nil || len(columns) == 0 || len(keyColumns) == 0 {
		return false
	}
	access, ok := NewVectorAccess(columns[0].Relation(), columns)
	if !ok || !state.addAccess(access) {
		return false
	}
	return state.addPhysicalVector(access, keyColumns, CoordinateClassLookupOnly)
}

func (state *deriveState) addScope(scope model.ScopeID) bool {
	if !scope.Available() {
		return false
	}
	if _, ok := state.scopeDefs[scope]; !ok || !bookScope(state.book, scope) {
		return false
	}
	return true
}

func (state *deriveState) addScopeVector(scope model.ScopeID) bool {
	definition, ok := state.scopeDefs[scope]
	if !ok {
		return false
	}
	return state.addColumnGroups(definition.Dimensions())
}

func (state *deriveState) addMappingVectors(mappings []algebra.ColumnMapping) bool {
	groups := make([]columnGroup, 0, len(mappings))
	for _, mapping := range mappings {
		source := mapping.Source()
		if !source.Available() {
			return false
		}
		found := -1
		for index := range groups {
			if groups[index].relation == source.Relation() {
				found = index
				break
			}
		}
		if found < 0 {
			groups = append(groups, columnGroup{relation: source.Relation()})
			found = len(groups) - 1
		}
		groups[found].columns = append(groups[found].columns, source)
	}
	for _, group := range groups {
		if !state.addVectorAccess(group.columns) {
			return false
		}
	}
	return true
}

// addProjectCorrespondence seals the source-side coordinates used to redeem
// a Project target key. The target KeySchema is already an indexed layout;
// this additional source vector is the exact-equality counterpart. It is
// kept in the existing Access/Layout/index census and can therefore be
// shared with a regular mapping vector when the authored vectors coincide.
// Physical indexing does not imply stable immutability: if a source mapping
// is a mutable payload, state/index consumes its before/after delta and
// updates this exact layout rather than applying the database stable fence.
func (state *deriveState) addProjectCorrespondence(mappings []algebra.ColumnMapping, key model.KeyID) bool {
	if state == nil || !key.Available() || mappings == nil {
		return false
	}
	keySchema, ok := state.keys[key]
	if !ok || !keySchema.Available() {
		return false
	}
	keyColumns := keySchema.Columns()
	if len(keyColumns) == 0 {
		return false
	}
	byRelation := make([]columnGroup, 0)
	for _, keyColumn := range keyColumns {
		found := false
		for _, mapping := range mappings {
			if mapping.Target() != keyColumn {
				continue
			}
			source := mapping.Source()
			if !source.Available() {
				return false
			}
			group := -1
			for index := range byRelation {
				if byRelation[index].relation == source.Relation() {
					group = index
					break
				}
			}
			if group < 0 {
				byRelation = append(byRelation, columnGroup{relation: source.Relation()})
				group = len(byRelation) - 1
			}
			byRelation[group].columns = append(byRelation[group].columns, source)
			found = true
			break
		}
		if !found {
			// The typing pass proves this correspondence before mount. Retain
			// a hard refusal here as defense in depth if a future contract
			// admits a target key without an exact source mapping.
			return false
		}
	}
	for _, group := range byRelation {
		if !state.addCorrespondenceAccess(group.columns) {
			return false
		}
	}
	return true
}

func (state *deriveState) addColumnGroups(columns []model.ColumnID) bool {
	groups := make([]columnGroup, 0, len(columns))
	for _, column := range columns {
		if !column.Available() {
			return false
		}
		found := -1
		for index := range groups {
			if groups[index].relation == column.Relation() {
				found = index
				break
			}
		}
		if found < 0 {
			groups = append(groups, columnGroup{relation: column.Relation()})
			found = len(groups) - 1
		}
		groups[found].columns = append(groups[found].columns, column)
	}
	for _, group := range groups {
		if !state.addVectorAccess(group.columns) {
			return false
		}
	}
	return true
}

func (state *deriveState) addAccess(access Access) bool {
	if !access.Available() {
		return false
	}
	if _, ok := state.relations[access.Relation()]; !ok {
		return false
	}
	if access.Key().Available() {
		if _, ok := state.keys[access.Key()]; !ok {
			return false
		}
	}
	for _, column := range access.Columns() {
		if !column.Available() || column.Relation() != access.Relation() {
			return false
		}
	}
	state.accesses = append(state.accesses, cloneAccess(access))
	return true
}

type columnGroup struct {
	relation model.RelationID
	columns  []model.ColumnID
}

func bookAccess(book address.Book, access Access) bool {
	if !access.Available() {
		return false
	}
	if value, ok := book.Relation(access.Relation()); !ok || !value.ValidFor(book.Fence()) {
		return false
	}
	if access.Key().Available() {
		if value, ok := book.Key(access.Key()); !ok || !value.ValidFor(book.Fence()) {
			return false
		}
	}
	for _, column := range access.Columns() {
		if value, ok := book.Column(column); !ok || !value.ValidFor(book.Fence()) {
			return false
		}
	}
	return true
}

func bookScope(book address.Book, scope model.ScopeID) bool {
	value, ok := book.Scope(scope)
	return ok && value.ValidFor(book.Fence())
}

func bookExpression(book address.Book, expression model.ExpressionID) bool {
	value, ok := book.Expression(expression)
	return ok && value.ValidFor(book.Fence())
}

func bookDependency(book address.Book, dependency model.DependencyID) bool {
	value, ok := book.Dependency(dependency)
	return ok && value.ValidFor(book.Fence())
}

func identityLess(left, right identity.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}

func sameColumns(left, right []model.ColumnID) bool {
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

// validatePartitionDirectories closes the cold mount catalog before any
// expression lowering. The certificate owns the logical partition authority;
// binding contributes only the opaque seal and already-fenced witnesses.
func validatePartitionDirectories(fence address.Fence, partitions []certificate.CorrelationPartition, directories []binding.PartitionDirectory) bool {
	if len(partitions) == 0 {
		return len(directories) == 0
	}
	if len(partitions) != len(directories) {
		return false
	}
	runtime, runtimeOK := binding.NewFence(fence.SchemaID(), fence.MountID(), fence.Generation())
	if !runtimeOK {
		return false
	}
	for _, partition := range partitions {
		if !partition.Available() {
			return false
		}
		matches := 0
		for _, directory := range directories {
			if !directory.Available() || !directory.ValidFor(runtime) || directory.Seal() != partition.Digest() || directory.Population() != partition.Population() || directory.Child() != partition.Child() {
				continue
			}
			matches++
		}
		if matches != 1 {
			return false
		}
	}
	return true
}
