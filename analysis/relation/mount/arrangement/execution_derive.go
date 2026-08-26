package arrangement

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// deriveExecution is the sole logical-to-physical expression lowering. It
// runs within Derive after every access has been resolved, never at runtime.
// Its resolver is immutable and cannot consult an inventory.
func deriveExecution(fence address.Fence, book address.Book, expressions []plan.ExpressionRef, recurrence certificate.RecurrenceData, relations []model.RelationSchema, layouts []Layout, deliveries []DeliveryRequirement, signatures []signature.Signature, expandEvidence expand.Catalog, partitionDirectories []binding.PartitionDirectory, partitionAuthorities []certificate.CorrelationPartition, proposalMerges map[identity.ContentID]bool) (Execution, bool) {
	// An expression-free schema has a valid authenticated empty execution
	// table.  The certificate already distinguishes it from an unavailable
	// plan; make below canonicalizes every empty vector before the table is
	// sealed.  Only the relation catalogue is required to exist at this
	// lowering boundary.
	if !fence.Available() || !book.Available() || !book.Fence().Same(fence) || relations == nil {
		return Execution{}, false
	}
	signatureIndex := make(map[signature.Identity]signature.Signature, len(signatures))
	for _, value := range signatures {
		if !value.Available() || !value.Identity().Available() {
			return Execution{}, false
		}
		if _, duplicate := signatureIndex[value.Identity()]; duplicate {
			return Execution{}, false
		}
		signatureIndex[value.Identity()] = value
	}
	resolver := executionResolver{
		fence: fence, book: book, relations: relations, layouts: layouts, deliveries: deliveries, signatures: signatureIndex, expandEvidence: expandEvidence, partitionDirectories: partitionDirectories, partitionAuthorities: partitionAuthorities, proposalMerges: proposalMerges,
		nodes: make(map[identity.ContentID]*executionNode), visiting: make(map[identity.ContentID]bool),
	}
	entries := make([]executionEntry, len(expressions))
	byID := make(map[model.ExpressionID]int, len(expressions))
	for index, expression := range expressions {
		if !expression.Available() || expression.Expression() == nil || !expression.ID().Available() {
			return Execution{}, false
		}
		if _, duplicate := byID[expression.ID()]; duplicate {
			return Execution{}, false
		}
		root, ok := resolver.node(expression.Expression())
		if !ok || root == nil {
			return Execution{}, false
		}
		derivationPlan, derivationOK := resolver.derivation(expression.ID(), expression.Expression(), signatures)
		if !derivationOK || !derivationPlan.Available() {
			return Execution{}, false
		}
		entries[index] = executionEntry{id: expression.ID(), digest: expression.Digest(), root: root, derivation: derivationPlan}
		byID[expression.ID()] = index
	}
	sort.SliceStable(entries, func(left, right int) bool { return compareExpression(entries[left].id, entries[right].id) < 0 })
	for index, entry := range entries {
		byID[entry.id] = index
	}
	dependencySchedule, scheduleOK := buildDependencySchedule(recurrence, entries)
	if !scheduleOK || !dependencySchedule.Available() {
		return Execution{}, false
	}
	byNode := make(map[identity.ContentID]*executionNode)
	byLogical := make(map[identity.ContentID]*executionNode)
	for _, entry := range entries {
		if !indexExecutionNodes(entry.root, byNode) {
			return Execution{}, false
		}
		if !indexLogicalNodes(entry.root, byLogical) {
			return Execution{}, false
		}
	}
	logicalParts := make([][]byte, 0, len(entries)*2)
	physicalParts := make([][]byte, 0, len(entries)*3)
	for _, entry := range entries {
		idPart := nominalBytes(entry.id.Owner().Content(), entry.id.Content())
		logicalParts = append(logicalParts, idPart, contentBytes(entry.digest))
		physicalParts = append(physicalParts, idPart, contentBytes(entry.digest), contentBytes(entry.root.digest), contentBytes(entry.derivation.Digest()))
	}
	logicalParts = append(logicalParts, contentBytes(dependencySchedule.LogicalDigest()))
	physicalParts = append(physicalParts, contentBytes(dependencySchedule.Digest()))
	logicalDigest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/execution/v1/logical", logicalParts...)
	if !ok {
		return Execution{}, false
	}
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/execution/v1", physicalParts...)
	if !ok {
		return Execution{}, false
	}
	data := &executionData{fence: fence, entries: entries, byID: byID, byNode: byNode, byLogical: byLogical, dependencies: dependencySchedule, logicalDigest: logicalDigest, digest: digest}
	if !validateExecution(data) {
		return Execution{}, false
	}
	data.sealed = true
	return Execution{data: data}, true
}

// indexExecutionNodes builds the one immutable physical-node directory used
// by path frames. Duplicate physical digests are permitted only when they
// name the same sealed node pointer; a collision across distinct nodes would
// make frame redemption ambiguous and therefore refuses the mount.
func indexExecutionNodes(node *executionNode, byNode map[identity.ContentID]*executionNode) bool {
	if node == nil || byNode == nil || !node.digest.Available() {
		return false
	}
	if prior, ok := byNode[node.digest]; ok {
		return prior == node
	}
	byNode[node.digest] = node
	for _, child := range node.children {
		if !indexExecutionNodes(child, byNode) {
			return false
		}
	}
	return true
}

func indexLogicalNodes(node *executionNode, byLogical map[identity.ContentID]*executionNode) bool {
	if node == nil || byLogical == nil || !node.logical.Available() {
		return false
	}
	if prior, ok := byLogical[node.logical]; ok {
		if prior != node {
			return false
		}
		return true
	}
	byLogical[node.logical] = node
	for _, child := range node.children {
		if !indexLogicalNodes(child, byLogical) {
			return false
		}
	}
	return true
}

type executionResolver struct {
	fence                address.Fence
	book                 address.Book
	relations            []model.RelationSchema
	layouts              []Layout
	deliveries           []DeliveryRequirement
	signatures           map[signature.Identity]signature.Signature
	expandEvidence       expand.Catalog
	partitionDirectories []binding.PartitionDirectory
	partitionAuthorities []certificate.CorrelationPartition
	proposalMerges       map[identity.ContentID]bool
	nodes                map[identity.ContentID]*executionNode
	visiting             map[identity.ContentID]bool
}

// derivation lowers one root into occurrence-specific immutable zippers after
// every layout has been resolved. The child package receives only sealed
// logical/physical evidence; it cannot reopen the address Book or retain this
// resolver for runtime use.
func (resolver *executionResolver) derivation(root model.ExpressionID, expression algebra.Expression, signatures []signature.Signature) (derivation.Plan, bool) {
	if resolver == nil || !root.Available() || expression == nil || resolver.relations == nil || resolver.layouts == nil {
		return derivation.Plan{}, false
	}
	bindings := make([]derivation.Binding, 0, len(resolver.layouts))
	seenAccesses := make([]Access, 0, len(resolver.layouts))
	for _, layout := range resolver.layouts {
		if !layout.Available() {
			return derivation.Plan{}, false
		}
		access := layout.Access()
		// Derivation binds one logical Access per occurrence vocabulary. A
		// row vector may have additional physical coordinates for Join/Merge/
		// Apply, but those are redeemed by the execution node's exact Layout;
		// they must not create duplicate logical Binding entries. Derive emits
		// the neutral vector first, so this deterministic representative keeps
		// ordinary Input leaves stable while the operator bindings select their
		// named coordinate class separately.
		duplicate := false
		for _, prior := range seenAccesses {
			if prior.Equal(access) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		seenAccesses = append(seenAccesses, cloneAccess(access))
		binding, ok := derivation.NewBinding(access.Relation(), access.Key(), access.Columns(), layout.Digest())
		if !ok {
			return derivation.Plan{}, false
		}
		bindings = append(bindings, binding)
	}
	inputExpressions, inputsOK := collectInputOccurrences(expression)
	if !inputsOK {
		return derivation.Plan{}, false
	}
	inputBindings := make([]derivation.InputBinding, 0, len(inputExpressions))
	for _, inputExpression := range inputExpressions {
		node, nodeOK := resolver.node(inputExpression)
		if !nodeOK || node == nil {
			return derivation.Plan{}, false
		}
		mounted := node.input
		if !mounted.Available() {
			return derivation.Plan{}, false
		}
		values := mounted.Values()
		columns := values.Columns()
		input, inputOK := derivation.NewInputBinding(inputExpression.Digest(), inputExpression.Relation(), columns, values.Digest())
		if !inputOK {
			return derivation.Plan{}, false
		}
		inputBindings = append(inputBindings, input)
	}
	if signatures == nil {
		signatures = []signature.Signature{}
	}
	result, ok := derivation.BuildWithExpand(root, expression, bindings, inputBindings, signatures, resolver.expandEvidence)
	if !ok {
		return derivation.Plan{}, false
	}
	return result, true
}

// collectInputOccurrences returns the exact Input nodes present in one
// expression tree. The digest is the occurrence identity; a repeated relation
// with two projections therefore produces two independent entries while a
// shared expression is bound once.
func collectInputOccurrences(expression algebra.Expression) ([]algebra.Input, bool) {
	if expression == nil {
		return nil, false
	}
	result := make([]algebra.Input, 0)
	seen := make(map[identity.ContentID]struct{})
	var visit func(algebra.Expression) bool
	visit = func(value algebra.Expression) bool {
		if value == nil || !value.Digest().Available() {
			return false
		}
		switch node := value.(type) {
		case algebra.Input:
			if !node.Available() {
				return false
			}
			if _, duplicate := seen[node.Digest()]; !duplicate {
				seen[node.Digest()] = struct{}{}
				result = append(result, node)
			}
			return true
		case *algebra.Input:
			return node != nil && visit(*node)
		case algebra.Select:
			return visit(node.Child())
		case *algebra.Select:
			return node != nil && visit(node.Child())
		case algebra.Project:
			return visit(node.Child())
		case *algebra.Project:
			return node != nil && visit(node.Child())
		case algebra.Join:
			return visit(node.Left()) && visit(node.Right())
		case *algebra.Join:
			return node != nil && visit(node.Left()) && visit(node.Right())
		case algebra.Merge:
			for _, child := range node.Inputs() {
				if !visit(child) {
					return false
				}
			}
			return true
		case *algebra.Merge:
			if node == nil {
				return false
			}
			return visit(algebra.Merge(*node))
		case algebra.Group:
			return visit(node.Child())
		case *algebra.Group:
			return node != nil && visit(node.Child())
		case algebra.Complete:
			return visit(node.Child())
		case *algebra.Complete:
			return node != nil && visit(node.Child())
		case algebra.Apply:
			for _, child := range node.Inputs() {
				if !visit(child) {
					return false
				}
			}
			return true
		case *algebra.Apply:
			if node == nil {
				return false
			}
			return visit(algebra.Apply(*node))
		case algebra.Publish:
			return visit(node.Child())
		case *algebra.Publish:
			return node != nil && visit(node.Child())
		case algebra.ColumnProject:
			return visit(node.Child())
		case *algebra.ColumnProject:
			return node != nil && visit(node.Child())
		case algebra.Expand:
			return visit(node.Child())
		case *algebra.Expand:
			return node != nil && visit(node.Child())
		default:
			return false
		}
	}
	if !visit(expression) {
		return nil, false
	}
	return result, true
}

func (resolver *executionResolver) node(expression algebra.Expression) (*executionNode, bool) {
	if resolver == nil || expression == nil || !expression.Digest().Available() {
		return nil, false
	}
	digest := expression.Digest()
	if existing, ok := resolver.nodes[digest]; ok {
		return existing, true
	}
	if resolver.visiting[digest] {
		return nil, false
	}
	resolver.visiting[digest] = true
	defer delete(resolver.visiting, digest)
	value := &executionNode{kind: expression.Kind(), logical: digest}
	var ok bool
	switch expressionValue := expression.(type) {
	case algebra.Input:
		value.input, ok = resolver.bindInput(expressionValue)
	case *algebra.Input:
		if expressionValue != nil {
			value.input, ok = resolver.bindInput(*expressionValue)
		}
	case algebra.Select:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.select_, ok = resolver.bindSelect(expressionValue)
		}
	case *algebra.Select:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.select_, ok = resolver.bindSelect(*expressionValue)
			}
		}
	case algebra.Project:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.project, ok = resolver.bindProject(expressionValue)
		}
	case *algebra.Project:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.project, ok = resolver.bindProject(*expressionValue)
			}
		}
	case algebra.ColumnProject:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.columnProject, ok = resolver.bindColumnProject(expressionValue)
		}
	case *algebra.ColumnProject:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.columnProject, ok = resolver.bindColumnProject(*expressionValue)
			}
		}
	case algebra.Expand:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.expand, ok = resolver.bindExpand(expressionValue)
		}
	case *algebra.Expand:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.expand, ok = resolver.bindExpand(*expressionValue)
			}
		}
	case algebra.Join:
		value.children, ok = resolver.children(expressionValue.Left(), expressionValue.Right())
		if ok {
			value.join, ok = resolver.bindJoin(expressionValue)
		}
	case *algebra.Join:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Left(), expressionValue.Right())
			if ok {
				value.join, ok = resolver.bindJoin(*expressionValue)
			}
		}
	case algebra.Merge:
		value.children, ok = resolver.children(expressionValue.Inputs()...)
		if ok {
			value.merge, ok = resolver.bindMerge(expressionValue, value.children)
		}
	case *algebra.Merge:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Inputs()...)
			if ok {
				value.merge, ok = resolver.bindMerge(*expressionValue, value.children)
			}
		}
	case algebra.Group:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.group, ok = resolver.bindGroup(expressionValue)
		}
	case *algebra.Group:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.group, ok = resolver.bindGroup(*expressionValue)
			}
		}
	case algebra.Complete:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.complete, ok = resolver.bindComplete(expressionValue)
		}
	case *algebra.Complete:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.complete, ok = resolver.bindComplete(*expressionValue)
			}
		}
	case algebra.Apply:
		inputs := expressionValue.Inputs()
		if len(inputs) == 0 {
			// Certificate typing admits this shape only as Publish's immediate
			// zero-input seed child. Keep its physical node childless: it
			// certifies the destination write but does not manufacture a tuple
			// source for the relational Apply evaluator.
			value.children = []*executionNode{}
			value.apply, ok = resolver.bindApply(expressionValue, value.children)
		} else {
			value.children, ok = resolver.children(inputs...)
			if ok {
				value.apply, ok = resolver.bindApply(expressionValue, value.children)
			}
		}
	case *algebra.Apply:
		if expressionValue != nil {
			inputs := expressionValue.Inputs()
			if len(inputs) == 0 {
				value.children = []*executionNode{}
				value.apply, ok = resolver.bindApply(*expressionValue, value.children)
			} else {
				value.children, ok = resolver.children(inputs...)
				if ok {
					value.apply, ok = resolver.bindApply(*expressionValue, value.children)
				}
			}
		}
	case algebra.Publish:
		value.children, ok = resolver.children(expressionValue.Child())
		if ok {
			value.publish, ok = resolver.bindPublish(expressionValue)
		}
	case *algebra.Publish:
		if expressionValue != nil {
			value.children, ok = resolver.children(expressionValue.Child())
			if ok {
				value.publish, ok = resolver.bindPublish(*expressionValue)
			}
		}
	}
	if !ok {
		return nil, false
	}
	value.cells, ok = executionCellLayout(value)
	if !ok || !value.cells.Available() || !value.cells.Digest().Available() {
		return nil, false
	}
	parts := [][]byte{contentBytes(digest)}
	parts = append(parts, contentBytes(value.cells.Digest()))
	for _, child := range value.children {
		parts = append(parts, contentBytes(child.digest))
	}
	for _, layout := range executionLayouts(*value) {
		parts = append(parts, contentBytes(layout.Digest()))
	}
	for _, column := range executionColumns(*value) {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	if value.kind == algebra.KindExpand && value.expand.Available() {
		parts = append(parts, contentBytes(value.expand.evidence.Digest()))
	}
	if value.kind == algebra.KindApply && value.apply.replay.Available() {
		// The sealed ApplyReplay carries the exact partition directory seals,
		// postings digest, and binding fence. Include it in the physical node
		// identity so changing any directory cannot leave an aliased execution
		// digest behind the mount boundary.
		parts = append(parts, contentBytes(value.apply.replay.Digest()))
	}
	value.digest, ok = identity.DeriveContentID("analysis/relation/mount/arrangement/execution/node/v1", parts...)
	available := executionNodeAvailable(value, resolver.fence, make(map[*executionNode]bool))
	if !ok || !available {
		return nil, false
	}
	resolver.nodes[digest] = value
	return value, true
}

func (resolver *executionResolver) children(expressions ...algebra.Expression) ([]*executionNode, bool) {
	if len(expressions) == 0 {
		return nil, false
	}
	result := make([]*executionNode, len(expressions))
	for index, expression := range expressions {
		value, ok := resolver.node(expression)
		if !ok {
			return nil, false
		}
		result[index] = value
	}
	return result, true
}

func (resolver *executionResolver) layout(access Access) (Layout, bool) {
	if resolver == nil || !access.Available() {
		return Layout{}, false
	}
	var result Layout
	found := false
	for _, layout := range resolver.layouts {
		if layout.Available() && layout.ValidFor(resolver.fence) && layout.Access().Equal(access) {
			if found {
				// A logical Access may have several physical coordinates.  A
				// caller that has not named the coordinate is ambiguous and must
				// refuse instead of depending on layout declaration order.
				return Layout{}, false
			}
			result = cloneLayout(layout)
			found = true
		}
	}
	return result, found
}

// layoutBy redeems one exact physical realization of a logical Access.  The
// coordinate class and ordered key vector are sealed arrangement facts; they
// are intentionally required at every multi-coordinate call site so a
// runtime consumer cannot silently select another index for the same row.
func (resolver *executionResolver) layoutBy(access Access, class CoordinateClass, keyColumns []model.ColumnID) (Layout, bool) {
	if resolver == nil || !access.Available() || !class.Available() {
		return Layout{}, false
	}
	var result Layout
	found := false
	for _, layout := range resolver.layouts {
		if !layout.Available() || !layout.ValidFor(resolver.fence) || !layout.Access().Equal(access) || layout.CoordinateClass() != class || keyColumns != nil && !sameColumnIDs(layout.KeyColumns(), keyColumns) {
			continue
		}
		if found {
			return Layout{}, false
		}
		result = cloneLayout(layout)
		found = true
	}
	return result, found
}

func (resolver *executionResolver) relation(relation model.RelationID) (Layout, bool) {
	access, ok := NewRelationAccess(relation)
	if !ok {
		return Layout{}, false
	}
	return resolver.layoutBy(access, CoordinateClassNone, nil)
}

// inputVector resolves Input's complete authored row vector.  An empty
// schema is legal when the checker admitted it: its vector is equal to the
// relation scan, while a nonempty schema receives its own vector layout.
func (resolver *executionResolver) inputVector(relation model.RelationID, columns []model.ColumnID) (Layout, bool) {
	access, ok := NewVectorAccess(relation, columns)
	if !ok {
		return Layout{}, false
	}
	return resolver.layoutBy(access, CoordinateClassNone, nil)
}

func (resolver *executionResolver) key(key model.KeyID) (Layout, bool) {
	access, ok := NewKeyAccess(key)
	if !ok {
		return Layout{}, false
	}
	return resolver.layoutBy(access, CoordinateClassDeclaredKey, nil)
}

// deliveryLayout redeems the exact logical Access issued by the signature
// input.  Its key presence is part of that Access identity: a keyed delivery
// uses the declared-key coordinate, while an unkeyed delivery uses the
// ordinary neutral coordinate.  The delivered column vector is not enough to
// reconstruct this choice and must never be used as a selector.
func (resolver *executionResolver) deliveryLayout(access Access) (Layout, bool) {
	if !access.Available() {
		return Layout{}, false
	}
	if access.Key().Available() {
		return resolver.layoutBy(access, CoordinateClassDeclaredKey, nil)
	}
	return resolver.layoutBy(access, CoordinateClassNone, nil)
}

func (resolver *executionResolver) vector(columns []model.ColumnID) (Layout, bool) {
	if len(columns) == 0 {
		return Layout{}, false
	}
	access, ok := NewVectorAccess(columns[0].Relation(), columns)
	if !ok {
		return Layout{}, false
	}
	// Exact correspondence vectors are the physical source for Join and
	// Project key mappings. If none was declared, use the neutral vector used
	// by ordinary projection/publication consumers.
	if stable, stableOK := resolver.layoutBy(access, CoordinateClassStableCorrespondence, columns); stableOK {
		return stable, true
	}
	return resolver.layoutBy(access, CoordinateClassNone, nil)
}

func (resolver *executionResolver) lookupVector(columns, keyColumns []model.ColumnID) (Layout, bool) {
	if len(columns) == 0 || len(keyColumns) == 0 {
		return Layout{}, false
	}
	access, ok := NewVectorAccess(columns[0].Relation(), columns)
	if !ok {
		return Layout{}, false
	}
	return resolver.layoutBy(access, CoordinateClassLookupOnly, keyColumns)
}

// populationDriver redeems the one sealed unkeyed coordinate projection
// declared by a correlated Apply's independent population. The owner-issued
// denominator supplies RowIDs; unrelated relation columns are deliberately
// excluded so derived payload cannot gate population membership.
func (resolver *executionResolver) populationDriver(population model.DenominatorRef, coordinate model.ColumnID) (Layout, bool) {
	if resolver == nil || !population.Available() || !coordinate.Available() || coordinate.Relation() != population.Relation() {
		return Layout{}, false
	}
	columns, ok := resolver.authoredColumns(population.Relation())
	if !ok || !containsColumn(columns, coordinate) {
		return Layout{}, false
	}
	access, ok := NewVectorAccess(population.Relation(), []model.ColumnID{coordinate})
	if !ok {
		return Layout{}, false
	}
	layout, layoutOK := resolver.layoutBy(access, CoordinateClassNone, nil)
	if !layoutOK || layout.CoordinateClass() != CoordinateClassNone || layout.KeyWidth() != 0 || layout.Access().Key().Available() {
		return Layout{}, false
	}
	return layout, true
}

// relationColumns redeems the exact typed row contract already checked in the
// certificate. Complete binds this vector once at mount; no operator may
// reconstruct it later by scanning a mounted column catalogue.
func (resolver *executionResolver) relationColumns(relation model.RelationID) ([]model.ColumnID, bool) {
	columns, ok := resolver.authoredColumns(relation)
	return columns, ok && len(columns) != 0
}

// authoredColumns redeems the one relation declaration captured by the
// certificate.  Input is the sole consumer allowed to accept an empty vector;
// Complete and Project retain their nonempty contracts through relationColumns.
func (resolver *executionResolver) authoredColumns(relation model.RelationID) ([]model.ColumnID, bool) {
	if resolver == nil || !relation.Available() || resolver.relations == nil {
		return nil, false
	}
	var columns []model.ColumnID
	found := false
	for _, schema := range resolver.relations {
		if !schema.Available() || !schema.ID().Available() {
			return nil, false
		}
		if schema.ID() != relation {
			continue
		}
		if found {
			return nil, false
		}
		found = true
		columns = schema.Columns()
	}
	if !found {
		return nil, false
	}
	seen := make(map[model.ColumnID]struct{}, len(columns))
	for _, column := range columns {
		if !column.Available() || column.Relation() != relation {
			return nil, false
		}
		if _, duplicate := seen[column]; duplicate {
			return nil, false
		}
		seen[column] = struct{}{}
	}
	return columns, true
}

func (resolver *executionResolver) bindInput(value algebra.Input) (InputBinding, bool) {
	if resolver == nil || !value.Available() {
		return InputBinding{}, false
	}
	scan, scanOK := resolver.relation(value.Relation())
	columns := value.Columns()
	if value.IsAllColumns() {
		var columnsOK bool
		columns, columnsOK = resolver.authoredColumns(value.Relation())
		if !columnsOK {
			return InputBinding{}, false
		}
	}
	values, valuesOK := resolver.inputVector(value.Relation(), columns)
	binding := InputBinding{relation: value.Relation(), scan: scan, values: values, sealed: scanOK && valuesOK}
	return binding, binding.Available()
}
func (resolver *executionResolver) bindSelect(value algebra.Select) (SelectBinding, bool) {
	scope, ok := resolver.book.Scope(value.Contract().Scope())
	binding := SelectBinding{scope: scope}
	return binding, ok && binding.ValidFor(resolver.fence)
}
func (resolver *executionResolver) bindProject(value algebra.Project) (ProjectBinding, bool) {
	contract := value.Contract()
	targetColumns, ok := resolver.relationColumns(contract.Target())
	if !ok {
		return ProjectBinding{}, false
	}
	// Target is the complete destination row vector. The separately bound Key
	// remains the equality/index contract; a Key layout has no delivered cells
	// and therefore cannot authenticate ProjectInto's destination tuple.
	target, ok := resolver.inputVector(contract.Target(), targetColumns)
	if !ok {
		return ProjectBinding{}, false
	}
	key, ok := resolver.key(contract.Key())
	if !ok {
		return ProjectBinding{}, false
	}
	mappings := contract.Mappings()
	if mappings == nil {
		mappings = []algebra.ColumnMapping{}
	}
	groups, ok := projectionGroups(mappings)
	if !ok {
		return ProjectBinding{}, false
	}
	layouts := make([]Layout, len(groups))
	for index, group := range groups {
		if layouts[index], ok = resolver.vector(group.columns); !ok {
			return ProjectBinding{}, false
		}
	}
	keyColumns := key.KeyColumns()
	keyGroups, keyGroupOK := projectionKeyGroups(mappings, keyColumns)
	if !keyGroupOK {
		return ProjectBinding{}, false
	}
	keyLayouts := make([]Layout, len(keyGroups))
	for index, group := range keyGroups {
		if keyLayouts[index], ok = resolver.vector(group.columns); !ok {
			return ProjectBinding{}, false
		}
		if !sameColumnIDs(keyLayouts[index].KeyColumns(), keyLayouts[index].Columns()) {
			return ProjectBinding{}, false
		}
	}
	bound := make([]ProjectionMapping, len(mappings))
	keyTargetGroup := make(map[model.ColumnID]int, len(keyColumns))
	for groupIndex, group := range keyGroups {
		for _, source := range group.columns {
			for _, mapping := range mappings {
				if mapping.Source() == source {
					keyTargetGroup[mapping.Target()] = groupIndex
					break
				}
			}
		}
	}
	seenTargets := make(map[model.ColumnID]struct{}, len(mappings))
	targetMembers := make(map[model.ColumnID]struct{}, len(targetColumns))
	for _, column := range targetColumns {
		if !column.Available() || column.Relation() != contract.Target() {
			return ProjectBinding{}, false
		}
		targetMembers[column] = struct{}{}
	}
	for index, mapping := range mappings {
		if !mapping.Source().Available() || !mapping.Target().Available() || mapping.Target().Relation() != contract.Target() {
			return ProjectBinding{}, false
		}
		if _, duplicate := seenTargets[mapping.Target()]; duplicate {
			return ProjectBinding{}, false
		}
		if _, member := targetMembers[mapping.Target()]; !member {
			return ProjectBinding{}, false
		}
		seenTargets[mapping.Target()] = struct{}{}
		group := -1
		for position, candidate := range groups {
			if candidate.relation == mapping.Source().Relation() {
				group = position
				break
			}
		}
		if group < 0 {
			return ProjectBinding{}, false
		}
		layout := layouts[group]
		if keyGroup, isKey := keyTargetGroup[mapping.Target()]; isKey {
			layout = keyLayouts[keyGroup]
		}
		bound[index] = ProjectionMapping{source: mapping.Source(), target: mapping.Target(), layout: layout}
	}
	keyOrder := make([]uint32, len(keyColumns))
	for keyIndex, keyColumn := range keyColumns {
		found := -1
		for mappingIndex, mapping := range bound {
			if mapping.target == keyColumn {
				found = mappingIndex
				break
			}
		}
		if found < 0 {
			return ProjectBinding{}, false
		}
		keyOrder[keyIndex] = uint32(found)
	}
	result, sealed := sealProjectBinding(ProjectBinding{target: target, key: key, mappings: bound, keyOrder: keyOrder})
	return result, sealed && result.Available()
}
func (resolver *executionResolver) bindJoin(value algebra.Join) (JoinBinding, bool) {
	contract := value.Contract()
	left, ok := resolver.vector(contract.LeftColumns())
	if !ok {
		return JoinBinding{}, false
	}
	right, ok := resolver.vector(contract.RightColumns())
	if !ok {
		return JoinBinding{}, false
	}
	return NewJoinBinding(left, right)
}
func (resolver *executionResolver) bindMerge(value algebra.Merge, children []*executionNode) (MergeBinding, bool) {
	key, ok := resolver.key(value.Contract().Key())
	if !ok || len(children) == 0 {
		return MergeBinding{}, false
	}
	proposalMerge, classified := resolver.proposalMerges[value.Digest()]
	if !classified {
		return MergeBinding{}, false
	}
	proposals := make([]ProposalWitness, 0)
	for _, child := range children {
		operations, operationsOK := proposalOperations(child)
		if !operationsOK {
			return MergeBinding{}, false
		}
		for _, operation := range operations {
			proposals = append(proposals, ProposalWitness{child: child.digest, operation: operation})
		}
	}
	if proposalMerge != (len(proposals) != 0) {
		return MergeBinding{}, false
	}
	result := MergeBinding{key: key, proposals: proposals}
	return result, ok && result.Available()
}

// proposalOperations follows only physical operators whose evaluator ABI
// preserves Apply sidecars. It is deliberately not a recursive syntax scan:
// Select/Join/Project/Group/Complete terminate the capability.
func proposalOperations(node *executionNode) ([]signature.Identity, bool) {
	if node == nil {
		return nil, false
	}
	switch node.kind {
	case algebra.KindApply:
		if !node.apply.Available() {
			return nil, false
		}
		return []signature.Identity{node.apply.Operation()}, true
	case algebra.KindColumnProject:
		if !node.columnProject.Available() || len(node.children) != 1 {
			return nil, false
		}
		return proposalOperations(node.children[0])
	case algebra.KindMerge:
		if !node.merge.Available() {
			return nil, false
		}
		return node.merge.ProposalOperations(), true
	default:
		return nil, true
	}
}
func (resolver *executionResolver) bindGroup(value algebra.Group) (GroupBinding, bool) {
	key, ok := resolver.key(value.Contract().Key())
	result := GroupBinding{key: key, cardinality: value.Contract().Cardinality()}
	return result, ok && result.Available()
}
func (resolver *executionResolver) bindComplete(value algebra.Complete) (CompleteBinding, bool) {
	denominator := value.Denominator()
	key, ok := resolver.key(denominator.Key())
	columns, columnsOK := resolver.relationColumns(denominator.Relation())
	if !ok || !columnsOK {
		return CompleteBinding{}, false
	}
	return newCompleteBinding(denominator, key, columns)
}
func (resolver *executionResolver) bindColumnProject(value algebra.ColumnProject) (ColumnProjectBinding, bool) {
	slots := value.Contract().Slots()
	if len(slots) == 0 {
		return ColumnProjectBinding{}, false
	}
	columns := make([]model.ColumnID, len(slots))
	for index, slot := range slots {
		columns[index] = slot.Column()
	}
	values, ok := resolver.vector(columns)
	result := ColumnProjectBinding{values: values, slots: append([]algebra.ColumnSlot(nil), slots...)}
	return result, ok && result.Available()
}

func (resolver *executionResolver) bindExpand(value algebra.Expand) (ExpandBinding, bool) {
	contract := value.Contract()
	if !contract.Available() {
		return ExpandBinding{}, false
	}
	evidence, evidenceOK := resolver.expandEvidence.At(value.Digest())
	if !evidenceOK || !evidence.Available() || evidence.Contract() != contract {
		return ExpandBinding{}, false
	}
	candidateColumns, candidateOK := resolver.authoredColumns(contract.Candidate())
	readerColumns, readerOK := resolver.authoredColumns(contract.Reader())
	if !candidateOK || !readerOK || len(candidateColumns) == 0 || len(readerColumns) == 0 {
		return ExpandBinding{}, false
	}
	candidate, candidateLayoutOK := resolver.inputVector(contract.Candidate(), candidateColumns)
	// Reader is consumed through keyed Lookup. It may share its logical
	// authored vector with an ordinary Input layout, so redeem the explicit
	// lookup coordinate rather than selecting by declaration order.
	reader, readerLayoutOK := resolver.lookupVector(readerColumns, []model.ColumnID{contract.Key()})
	key, keyOK := resolver.keyForColumn(contract.Key())
	scope, scopeOK := resolver.book.Scope(contract.Scope())
	if !candidateLayoutOK || !readerLayoutOK || !keyOK || !scopeOK {
		return ExpandBinding{}, false
	}
	columns := append(append([]model.ColumnID(nil), candidateColumns...), readerColumns...)
	result := ExpandBinding{contract: contract, scope: scope, candidate: candidate, reader: reader, key: key, evidence: evidence, columns: columns}
	return result, result.Available()
}

func (resolver *executionResolver) keyForColumn(column model.ColumnID) (Layout, bool) {
	if resolver == nil || !column.Available() {
		return Layout{}, false
	}
	var result Layout
	for _, layout := range resolver.layouts {
		if !layout.Available() || !layout.Access().Key().Available() || layout.Access().Relation() != column.Relation() || !containsColumn(layout.KeyColumns(), column) {
			continue
		}
		if result.Available() {
			return Layout{}, false
		}
		result = cloneLayout(layout)
	}
	return result, result.Available()
}
func (resolver *executionResolver) bindApply(value algebra.Apply, children []*executionNode) (ApplyBinding, bool) {
	operation := value.Contract().Operation()
	cells, cellsOK := resolver.applyResultCellLayout(operation)
	if !cellsOK {
		return ApplyBinding{}, false
	}
	bound := make([]DeliveryBinding, 0)
	for _, requirement := range resolver.deliveries {
		if requirement.Operation() != operation {
			continue
		}
		access, ok := requirement.Access()
		if !ok {
			return ApplyBinding{}, false
		}
		layout, ok := resolver.deliveryLayout(access)
		if !ok {
			return ApplyBinding{}, false
		}
		var order Layout
		if requirement.Delivery().IsSpan() {
			if order, ok = resolver.key(requirement.Delivery().OrderKey()); !ok {
				return ApplyBinding{}, false
			}
		}
		bound = append(bound, DeliveryBinding{requirement: requirement, layout: layout, order: order})
	}
	sort.SliceStable(bound, func(left, right int) bool { return bound[left].requirement.Index() < bound[right].requirement.Index() })
	if bound == nil {
		bound = []DeliveryBinding{}
	}
	slotSource := value.Contract().SlotSource()
	childCount, groupsOK := denseSlotSource(slotSource)
	if !groupsOK {
		return ApplyBinding{}, false
	}
	output := value.Contract().Output()
	if !output.Available() {
		return ApplyBinding{}, false
	}
	outputSlot := -1
	if !output.IsOwnerNamed() {
		source, sourceOK := output.Source()
		if !sourceOK {
			return ApplyBinding{}, false
		}
		for index, candidate := range slotSource {
			if candidate != source {
				continue
			}
			if outputSlot != -1 {
				// A destination source must resolve to one operation slot; an
				// ambiguous repeated occurrence would make mount choose a row.
				return ApplyBinding{}, false
			}
			outputSlot = index
		}
		if outputSlot == -1 {
			return ApplyBinding{}, false
		}
		if outputSlot >= len(bound) {
			return ApplyBinding{}, false
		}
		input := bound[outputSlot].Requirement().Input()
		if output.IsScalarSource() && !input.Delivery.IsScalar() || output.IsSpanSource() && !input.Delivery.IsComplete() {
			return ApplyBinding{}, false
		}
	}
	correlation := value.Contract().Correlation()
	if correlation.Specified() {
		if !correlation.Available() || len(children) != correlation.ProjectionCount() || len(children) < 2 {
			return ApplyBinding{}, false
		}
		driver, driverOK := resolver.populationDriver(correlation.Population(), correlation.Coordinate())
		if !driverOK {
			return ApplyBinding{}, false
		}
		subtrees := make([]CorrelatedSubtree, len(children))
		for index, child := range children {
			projection, projectionOK := correlation.ProjectionAt(index)
			if child == nil || !projectionOK || len(projection) > 1 {
				return ApplyBinding{}, false
			}
			var partition certificate.CorrelationPartition
			var directory binding.PartitionDirectory
			if len(projection) == 1 {
				if carrier, carrierOK := correlatedCarrierDenominator(child); carrierOK {
					var directoryOK bool
					partition, directory, directoryOK = resolver.partitionForCarrier(value, uint32(index), carrier, correlation)
					if !directoryOK {
						return ApplyBinding{}, false
					}
				}
			}
			var subtreeOK bool
			subtrees[index], subtreeOK = sealCorrelatedSubtree(uint32(index), child, correlation, driver, slotSource, bound, partition, directory)
			if !subtreeOK || !subtrees[index].Available() {
				return ApplyBinding{}, false
			}
		}
		replay, replayOK := newApplyReplay(value.Digest(), correlation, driver, subtrees)
		if !replayOK || !replay.Available() {
			return ApplyBinding{}, false
		}
		result := ApplyBinding{operation: operation, deliveries: bound, slotSource: slotSource, cells: cells, output: output, outputSlot: outputSlot, childCount: uint32(childCount), correlation: correlation, replay: replay}
		return result, result.Available() && resolver.validateApplyCellLayout(children, result)
	}
	result := ApplyBinding{operation: operation, deliveries: bound, slotSource: slotSource, cells: cells, output: output, outputSlot: outputSlot, childCount: uint32(childCount)}
	return result, result.Available() && resolver.validateApplyCellLayout(children, result)
}

// applyResultCellLayout freezes Apply's result vector from the exact mounted
// signature. It is the one place mount reads that declaration: nested nodes
// subsequently redeem ApplyBinding.OutputCells rather than reopening a
// signature catalogue at runtime.
func (resolver *executionResolver) applyResultCellLayout(operation signature.Identity) (algebra.CellLayout, bool) {
	if resolver == nil || !operation.Available() {
		return algebra.CellLayout{}, false
	}
	semantic, semanticOK := resolver.signatures[operation]
	if !semanticOK || !semantic.Available() || semantic.Identity() != operation {
		return algebra.CellLayout{}, false
	}
	outputs := semantic.Outputs()
	if len(outputs) == 0 || !outputs[0].Available() || !outputs[0].Relation.Available() {
		return algebra.CellLayout{}, false
	}
	relation := outputs[0].Relation
	columns := make([]model.ColumnID, len(outputs))
	for index, output := range outputs {
		if !output.Available() || output.Relation != relation || output.Column.Relation() != relation {
			return algebra.CellLayout{}, false
		}
		columns[index] = output.Column
	}
	return applyOutputCellLayout(columns, relation)
}

// validateApplyCellLayout independently redeems every SlotSource against the
// already-sealed child output layouts. This is deliberately a mount-time
// proof: a malformed compiler layout refuses before an evaluator can read a
// wrong tuple cell, and no runtime ordinal adjustment is introduced.
func (resolver *executionResolver) validateApplyCellLayout(children []*executionNode, bound ApplyBinding) bool {
	if resolver == nil || !bound.Available() || len(children) != bound.ChildCount() {
		return false
	}
	semantic, semanticOK := resolver.signatures[bound.Operation()]
	if !semanticOK || !semantic.Available() || semantic.Identity() != bound.Operation() || semantic.InputLen() != len(bound.slotSource) {
		return false
	}
	expected, expectedOK := resolver.applyResultCellLayout(bound.Operation())
	if !expectedOK || !expected.Equal(bound.cells) {
		return false
	}
	for index, source := range bound.slotSource {
		child := int(source.Child())
		if child < 0 || child >= len(children) || children[child] == nil || !children[child].cells.Available() {
			return false
		}
		input, inputOK := semantic.InputAt(index)
		cell, cellOK := children[child].cells.CellAt(int(source.Cell()))
		if !inputOK || !cellOK || cell.Column() != input.Column {
			return false
		}
		relation, relationOK := children[child].cells.SourceAt(int(cell.Source()))
		if !relationOK || relation != input.Relation {
			return false
		}
	}
	return true
}

// partitionForCarrier joins one checked certificate partition to its already
// issued opaque posting directory. The authority stays carrier-only: source
// leaves in a joined subtree are sealed by their own mounted denominators and
// must never acquire a second Q partition.
func (resolver *executionResolver) partitionForCarrier(value algebra.Apply, ordinal uint32, carrier model.DenominatorRef, correlation algebra.ApplyCorrelation) (certificate.CorrelationPartition, binding.PartitionDirectory, bool) {
	if resolver == nil || !carrier.Available() || !value.Digest().Available() || !correlation.Available() || ordinal >= uint32(correlation.ProjectionCount()) || resolver.partitionAuthorities == nil || resolver.partitionDirectories == nil {
		return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
	}
	projection, projectionOK := correlation.ProjectionAt(int(ordinal))
	if !projectionOK || len(projection) != 1 {
		return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
	}
	var authority certificate.CorrelationPartition
	foundAuthority := false
	for _, candidate := range resolver.partitionAuthorities {
		if !candidate.Available() || candidate.Apply() != value.Digest() || candidate.Ordinal() != ordinal || candidate.Population() != correlation.Population() || candidate.Child() != carrier || candidate.Projection() != projection[0] {
			continue
		}
		if foundAuthority {
			return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
		}
		authority = candidate
		foundAuthority = true
	}
	if !foundAuthority {
		return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
	}
	var directory binding.PartitionDirectory
	foundDirectory := false
	for _, candidate := range resolver.partitionDirectories {
		if !candidate.Available() || candidate.Seal() != authority.Digest() || candidate.Population() != authority.Population() || candidate.Child() != authority.Child() {
			continue
		}
		if foundDirectory {
			return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
		}
		directory = candidate
		foundDirectory = true
	}
	if !foundDirectory {
		return certificate.CorrelationPartition{}, binding.PartitionDirectory{}, false
	}
	return authority, directory, true
}

func denseSlotSource(values []algebra.SlotSource) (int, bool) {
	if len(values) == 0 {
		// Only a certificate-admitted zero-input seed reaches this binding;
		// ordinary Apply nodes retain their non-empty slot-source contract in
		// typing. The mounted node is a publication certificate, not an
		// executable tuple product.
		return 0, true
	}
	max := uint32(0)
	for _, value := range values {
		if value.Child() > max {
			max = value.Child()
		}
	}
	count := int(max) + 1
	for expected := uint32(0); expected <= max; expected++ {
		found := false
		for _, value := range values {
			if value.Child() == expected {
				found = true
				break
			}
		}
		if !found {
			return 0, false
		}
	}
	return count, true
}
func (resolver *executionResolver) bindPublish(value algebra.Publish) (PublishBinding, bool) {
	contract := value.Contract()
	destination, ok := resolver.relation(contract.Destination())
	if !ok {
		return PublishBinding{}, false
	}
	key, ok := resolver.key(contract.Key())
	if !ok {
		return PublishBinding{}, false
	}
	columns := contract.Columns()
	if len(columns) == 0 {
		var columnsOK bool
		columns, columnsOK = resolver.relationColumns(contract.Destination())
		if !columnsOK {
			return PublishBinding{}, false
		}
	}
	writable, writableOK := resolver.vector(columns)
	result := PublishBinding{destination: destination, key: key, columns: writable}
	return result, writableOK && result.Available()
}

func projectionGroups(mappings []algebra.ColumnMapping) ([]columnGroup, bool) {
	result := make([]columnGroup, 0, len(mappings))
	for _, mapping := range mappings {
		source := mapping.Source()
		if !source.Available() {
			return nil, false
		}
		found := -1
		for index := range result {
			if result[index].relation == source.Relation() {
				found = index
				break
			}
		}
		if found < 0 {
			result = append(result, columnGroup{relation: source.Relation()})
			found = len(result) - 1
		}
		result[found].columns = append(result[found].columns, source)
	}
	return result, true
}

// projectionKeyGroups derives the source-side vectors for the exact target
// key correspondence. The target key order is authoritative; source groups
// preserve that order within each source relation so resolver.vector reaches
// the keyed physical Access sealed by deriveState.addProjectCorrespondence.
func projectionKeyGroups(mappings []algebra.ColumnMapping, keyColumns []model.ColumnID) ([]columnGroup, bool) {
	if mappings == nil || len(keyColumns) == 0 {
		return nil, false
	}
	result := make([]columnGroup, 0, len(keyColumns))
	for _, target := range keyColumns {
		if !target.Available() {
			return nil, false
		}
		found := false
		for _, mapping := range mappings {
			if mapping.Target() != target {
				continue
			}
			source := mapping.Source()
			if !source.Available() {
				return nil, false
			}
			group := -1
			for index := range result {
				if result[index].relation == source.Relation() {
					group = index
					break
				}
			}
			if group < 0 {
				result = append(result, columnGroup{relation: source.Relation()})
				group = len(result) - 1
			}
			result[group].columns = append(result[group].columns, source)
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}
	return result, true
}
