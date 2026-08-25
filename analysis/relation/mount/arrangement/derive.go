package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const planDigestDomain = "analysis/relation/mount/arrangement/plan/v1"

// Derive resolves every logical access required by a checked certificate.  It
// consumes only the opaque certificate and a fenced address Book; declarations
// and checker subpasses never cross this boundary.  Access requirements are
// canonicalized before Inventory is consulted, so declaration order cannot
// affect resolver calls or the logical plan.
func Derive(cert certificate.Certificate, book address.Book, inventory Inventory) (Plan, bool) {
	if inventory == nil || !cert.Available() || !book.Available() {
		return Plan{}, false
	}
	fence := inventory.Fence()
	if !fence.Available() || !fence.Same(book.Fence()) || fence.SchemaID() != cert.SchemaID() || fence.CertificateDigest() != cert.Digest() {
		return Plan{}, false
	}
	state := deriveState{
		book:       book,
		relations:  make(map[model.RelationID]model.RelationSchema),
		keys:       make(map[model.KeyID]model.KeySchema),
		scopeDefs:  make(map[model.ScopeID]model.ScopeSchema),
		signatures: make(map[signature.Identity]signature.Signature),
	}
	for _, value := range cert.Relations() {
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
	layouts := make([]Layout, len(state.accesses))
	seenHandles := make(map[Handle]struct{}, len(state.accesses))
	for index, access := range state.accesses {
		handle, ok := inventory.Resolve(access)
		if !ok || !handle.Available() || !handle.ValidFor(fence) {
			return Plan{}, false
		}
		if _, duplicate := seenHandles[handle]; duplicate {
			return Plan{}, false
		}
		seenHandles[handle] = struct{}{}
		var keyColumns []model.ColumnID
		if access.Key().Available() {
			key, keyOK := state.keys[access.Key()]
			if !keyOK || !key.Available() {
				return Plan{}, false
			}
			keyColumns = key.Columns()
		}
		layout, layoutOK := newLayout(fence, handle, access, keyColumns)
		if !layoutOK {
			return Plan{}, false
		}
		layouts[index] = layout
	}

	data := &planData{
		fence:      fence,
		layouts:    layouts,
		deliveries: state.deliveries,
	}
	logicalParts := make([][]byte, 0, len(data.layouts)+len(data.deliveries))
	appendPlanDigestParts(&logicalParts, *data)
	logicalDigest, ok := identity.DeriveContentID(planDigestDomain+"/logical", logicalParts...)
	if !ok {
		return Plan{}, false
	}
	physicalParts := append([][]byte(nil), logicalParts...)
	for _, layout := range data.layouts {
		physicalParts = append(physicalParts, contentBytes(layout.Digest()))
	}
	digest, ok := identity.DeriveContentID(planDigestDomain, physicalParts...)
	if !ok {
		return Plan{}, false
	}
	data.logicalDigest = logicalDigest
	data.digest = digest
	return Plan{data: data}, true
}

type deriveState struct {
	book       address.Book
	relations  map[model.RelationID]model.RelationSchema
	keys       map[model.KeyID]model.KeySchema
	scopeDefs  map[model.ScopeID]model.ScopeSchema
	signatures map[signature.Identity]signature.Signature
	accesses   []Access
	deliveries []DeliveryRequirement
	visiting   map[identity.ContentID]bool
	visited    map[identity.ContentID]bool
}

func (state *deriveState) addSignature(value signature.Signature) bool {
	for index, input := range value.Inputs() {
		requirement, ok := newDeliveryRequirement(value.Identity(), index, input)
		if !ok || !state.addInputAccess(input) {
			return false
		}
		state.deliveries = append(state.deliveries, requirement)
	}
	for _, output := range value.Outputs() {
		if !output.Available() {
			return false
		}
		access, ok := NewVectorAccess(output.Relation, []model.ColumnID{output.Column})
		if !ok || !state.addAccess(access) {
			return false
		}
	}
	authority := value.Authority().Denominator
	if !authority.Available() || !state.addKeyAccess(authority.Key()) {
		return false
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
		ok = state.addRelationAccess(value.Relation())
	case *algebra.Input:
		ok = value != nil && state.addRelationAccess(value.Relation())
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
			ok = state.addRelationAccess(contract.Target()) && state.addKeyAccess(contract.Key())
			if ok {
				ok = state.addMappingVectors(contract.Mappings())
			}
		}
	case *algebra.Project:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				contract := value.Contract()
				ok = state.addRelationAccess(contract.Target()) && state.addKeyAccess(contract.Key()) && state.addMappingVectors(contract.Mappings())
			}
		}
	case algebra.Join:
		ok = state.deriveExpression(value.Left())
		if ok {
			ok = state.deriveExpression(value.Right())
		}
		if ok {
			contract := value.Contract()
			ok = state.addVectorAccess(contract.LeftColumns()) && state.addVectorAccess(contract.RightColumns())
		}
	case *algebra.Join:
		if value != nil {
			ok = state.deriveExpression(value.Left())
			if ok {
				ok = state.deriveExpression(value.Right())
			}
			if ok {
				contract := value.Contract()
				ok = state.addVectorAccess(contract.LeftColumns()) && state.addVectorAccess(contract.RightColumns())
			}
		}
	case algebra.Merge:
		ok = true
		for _, child := range value.Inputs() {
			ok = ok && state.deriveExpression(child)
		}
		if ok {
			ok = state.addKeyAccess(value.Contract().Key())
		}
	case *algebra.Merge:
		if value != nil {
			ok = true
			for _, child := range value.Inputs() {
				ok = ok && state.deriveExpression(child)
			}
			if ok {
				ok = state.addKeyAccess(value.Contract().Key())
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
		}
	case *algebra.Apply:
		if value != nil {
			ok = true
			for _, child := range value.Inputs() {
				ok = ok && state.deriveExpression(child)
			}
			if ok {
				_, ok = state.signatures[value.Contract().Operation()]
			}
		}
	case algebra.Publish:
		ok = state.deriveExpression(value.Child())
		if ok {
			contract := value.Contract()
			ok = state.addRelationAccess(contract.Destination()) && state.addKeyAccess(contract.Key())
		}
	case *algebra.Publish:
		if value != nil {
			ok = state.deriveExpression(value.Child())
			if ok {
				contract := value.Contract()
				ok = state.addRelationAccess(contract.Destination()) && state.addKeyAccess(contract.Key())
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
