package boundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// RequireOperation returns the one exact Target operation bound to the
// builtin require ingress. The false case means this Target has no scoped
// require authority; zero is never an admitted operation.
func (c *Component) RequireOperation() (vocabulary.Operation, bool) {
	if c == nil || c.authority == nil || c.authority.require == 0 {
		return 0, false
	}
	return c.authority.require, true
}

// MatchesProject is the hot exact-owner fence used by downstream components.
// Equivalent content is intentionally insufficient: the Project pointer is
// the authority that issued every Shard/Application consumed by Boundary.
func (c *Component) MatchesProject(project *linkproject.Component) bool {
	return c != nil && c.authority != nil && c.authority.component == c && project != nil && c.authority.project == project
}

// Target returns Boundary's exact sealed Target authority. It is available
// only from the finalized component that retains it; callers cannot rebind an
// equivalent contract by ContentID.
func (c *Component) Target() (*target.Contract, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || c.authority.target == nil {
		return nil, false
	}
	return c.authority.target, true
}

// InitialOperation reduces one exact Project key and one actor boot-root
// coordinate to the admitted Target operation at that root/key row.  The
// Boundary is the rightful owner of this projection: it already retains the
// exact Project and Target authorities, while Target alone cannot authenticate
// a Project key and Project alone must not interpret Target initial values.
//
// The Project key is first reduced through Project's sealed owner-local inverse
// and the Target row is then resolved through Target's allocation-free
// InitialEntry index.  Foreign equivalent reseals, same-ordinal handles,
// source-only keys, non-operation values, missing rows, and ambiguous inverse
// rows all fail closed.
func (c *Component) InitialOperation(project *linkproject.Component, contract *target.Contract, root vocabulary.InitialRoot, key linkproject.Key) (vocabulary.Operation, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || project == nil || project != c.authority.project || contract == nil || contract != c.authority.target || root == 0 {
		return 0, false
	}
	targetKey, ok := project.Keys().TargetFor(contract, key)
	if !ok {
		return 0, false
	}
	return contract.InitialOperation(root, targetKey)
}

// ModuleRelationID is the narrow Boundary projection consumed by Module.
// It commits only canonical Values and scoped-require disposition; endpoint,
// denied bootstrap, Host, Static, and enclosing Link geometry are absent.
func (c *Component) ModuleRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || !c.authority.moduleRelation.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.moduleRelation, true
}

// ValueRelationID identifies Boundary's canonical existing Program-value
// relation. It excludes Seed, Endpoint, Host, Module, Static, and Link state.
func (c *Component) ValueRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || c.authority.valueTable == nil || !c.authority.valueTable.content.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.valueTable.content, true
}

// EndpointRelationID identifies Boundary's canonical admitted endpoint
// relation. It excludes ordinary/loader/denied Seed families and all later
// Host, Module, Static, and Link state.
func (c *Component) EndpointRelationID() (identity.ContentID, bool) {
	if c == nil || c.authority == nil || c.authority.component != c || c.authority.seedTable == nil || !c.authority.seedTable.endpointRelation.Available() {
		return identity.ContentID{}, false
	}
	return c.authority.seedTable.endpointRelation, true
}

// Values returns Boundary's sole immutable Program-value authority.
func (c *Component) Values() Values {
	if c == nil || c.authority == nil || c.authority.component != c {
		return Values{}
	}
	return Values{component: c}
}

// Calls returns Boundary's exact ordinary-Call operand view. Project remains
// the owner of the Application coordinates themselves.
func (c *Component) Calls() Calls {
	if c == nil || c.authority == nil || c.authority.component != c {
		return Calls{}
	}
	return Calls{component: c}
}

// Seeds returns Boundary's finite external-value authority.
func (c *Component) Seeds() Seeds {
	if c == nil || c.authority == nil || c.authority.component != c {
		return Seeds{}
	}
	return Seeds{component: c}
}

// Endpoints returns Boundary's nominal provider endpoint authority.
func (c *Component) Endpoints() Endpoints {
	if c == nil || c.authority == nil || c.authority.component != c {
		return Endpoints{}
	}
	return Endpoints{component: c}
}

// EndpointRequests returns the canonical replay-only endpoint contract.
func (c *Component) EndpointRequests() EndpointRequests {
	if c == nil || c.authority == nil || c.authority.component != c {
		return EndpointRequests{}
	}
	return EndpointRequests{component: c}
}

// ApplicationOperationAvailable is the factorized LinkBoundary membership
// predicate. It validates both exact authorities and then classifies only the
// existing Project Application subsequences; it stores no product relation.
func (c *Component) ApplicationOperationAvailable(contract *target.Contract, application linkproject.Application, operation vocabulary.Operation) bool {
	if c == nil || c.authority == nil || c.authority.project == nil || contract == nil || contract != c.authority.target || operation == 0 || uint64(operation) > uint64(contract.OperationCount()) {
		return false
	}
	applications := c.authority.project.Applications()
	if _, _, _, imported := applications.Import(application); imported {
		return false
	}
	call := false
	if _, _, ok := applications.Call(application); ok {
		call = true
	} else if !projectBaseApplication(applications, application) {
		return false
	}
	// The scoped require loader is shard-local source geometry. Only an
	// ordinary Call application can name it; operator and generic applications
	// remain eligible for every other Target operation.
	if operation == c.authority.require {
		return call
	}
	return true
}

// Cardinality returns the exact mathematical cardinality of the virtual
// boundary predicate. It validates the typed Bases/Calls partition and then
// evaluates B*O, subtracting the non-Call bases for the one scoped require
// operation. No Application x Operation predicate is evaluated here and no
// product rows, maps, bitsets, or reverse indexes are retained.
func (c *Component) Cardinality() (int, bool) {
	if c == nil || c.authority == nil || c.authority.target == nil || c.authority.project == nil {
		return 0, false
	}
	applications := c.authority.project.Applications()
	bases := applications.Bases()
	calls := applications.Calls()
	if !validateBasePartition(applications, bases, calls) {
		return 0, false
	}
	operations := c.authority.target.OperationCount()
	if operations < 0 {
		return 0, false
	}
	for operationIndex := 0; operationIndex < operations; operationIndex++ {
		operation, ok := c.authority.target.OperationAt(operationIndex)
		if !ok || operation == 0 {
			return 0, false
		}
	}
	require, hasRequire := c.RequireOperation()
	if hasRequire && (require == 0 || uint64(require) > uint64(operations)) {
		return 0, false
	}
	return checkedCardinality(bases.Count(), calls.Count(), operations, hasRequire)
}

func validateBasePartition(applications linkproject.Applications, bases linkproject.Bases, calls linkproject.Calls) bool {
	baseCount := bases.Count()
	callCount := calls.Count()
	if baseCount < 0 || callCount < 0 || callCount > baseCount {
		return false
	}
	for index := 0; index < baseCount; index++ {
		application, ok := bases.At(index)
		if !ok || baseApplicationClassificationCount(applications, application) != 1 {
			return false
		}
		if index != 0 {
			prior, priorOK := bases.At(index - 1)
			order, orderOK := applications.Compare(prior, application)
			if !priorOK || !orderOK || order >= 0 {
				return false
			}
		}
	}
	for index := 0; index < callCount; index++ {
		application, ok := calls.At(index)
		if !ok {
			return false
		}
		if _, _, callOK := applications.Call(application); !callOK {
			return false
		}
		if index != 0 {
			prior, priorOK := calls.At(index - 1)
			order, orderOK := applications.Compare(prior, application)
			if !priorOK || !orderOK || order >= 0 {
				return false
			}
		}
	}
	// Calls must be an exact typed subsequence of Bases. Both sequences are
	// canonical, so a merge proves the partition in O(B+C), without a map or
	// BxC search.
	baseIndex, callIndex := 0, 0
	for callIndex < callCount {
		call, callOK := calls.At(callIndex)
		if !callOK {
			return false
		}
		matched := false
		for baseIndex < baseCount {
			base, baseOK := bases.At(baseIndex)
			order, orderOK := applications.Compare(base, call)
			if !baseOK || !orderOK {
				return false
			}
			if order < 0 {
				baseIndex++
				continue
			}
			if order != 0 {
				return false
			}
			baseIndex++
			callIndex++
			matched = true
			break
		}
		if !matched {
			return false
		}
	}
	return true
}

func baseApplicationClassificationCount(applications linkproject.Applications, application linkproject.Application) int {
	count := 0
	if _, _, ok := applications.Call(application); ok {
		count++
	}
	if _, _, ok := applications.Generic(application); ok {
		count++
	}
	operators := applications.Operators()
	checks := [...]func(linkproject.Application) (linkproject.Shard, keyspace.Term, bool){
		operators.UnaryNumeric,
		operators.Length,
		operators.Arithmetic,
		operators.Bitwise,
		operators.Concat,
		operators.Equality,
		operators.OrderPrimary,
		operators.OrderFallback,
		operators.IndexGet,
		operators.IndexSet,
	}
	for _, check := range checks {
		if _, _, ok := check(application); ok {
			count++
		}
	}
	return count
}

func checkedCardinality(baseCount, callCount, operationCount int, scopedRequire bool) (int, bool) {
	maximum := int(^uint(0) >> 1)
	if baseCount < 0 || callCount < 0 || operationCount < 0 || callCount > baseCount {
		return 0, false
	}
	if scopedRequire && operationCount == 0 {
		return 0, false
	}
	if baseCount != 0 && operationCount > maximum/baseCount {
		return 0, false
	}
	count := baseCount * operationCount
	if !scopedRequire {
		return count, true
	}
	excluded := baseCount - callCount
	if excluded > count {
		return 0, false
	}
	return count - excluded, true
}

// ContentID is the immutable identity of the factorized boundary topology.
// It names the exact Target and Project Application relation authorities and
// never a product row or derived require ordinal.
func (c *Component) ContentID() (id identity.ContentID) {
	if c == nil || c.authority == nil {
		return identity.ContentID{}
	}
	return c.authority.content
}

func projectBaseApplication(applications linkproject.Applications, application linkproject.Application) bool {
	return baseApplicationClassificationCount(applications, application) == 1
}
