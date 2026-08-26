package terminal

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Application is one immutable terminal observation redeemed for one exact
// mounted observation declaration. The composite identity is intentional:
// one dependency may contain several sealed Apply occurrences, and operation
// identity remains available even when Result.Len() is zero.
//
// The catalog retains only Apply's typed result extent and its observed root.
// Relation batches, evaluator results, publication settlements, and any
// second row/query store stop at the runtime boundary.
type Application struct {
	dependency model.DependencyID
	operation  signature.Identity
	root       database.Version
	result     apply.Results
	sealed     bool
}

// NewApplication seals one observed Apply extent. The operation is checked
// twice: against the exact catalog key and against Results' own operation
// identity. This preserves an empty authenticated extent without allowing a
// caller to pair a valid result with a foreign declaration.
func NewApplication(root database.Version, dependency model.DependencyID, operation signature.Identity, result apply.Results) (Application, bool) {
	if !root.Available() || !dependency.Available() || !operation.Available() || !result.Available() || result.Operation() != operation {
		return Application{}, false
	}
	if !resultValidFor(root, operation, result) {
		return Application{}, false
	}
	value := Application{
		dependency: dependency,
		operation:  operation,
		root:       root,
		result:     result,
		sealed:     true,
	}
	return value, value.Available()
}

// Available authenticates the complete immutable observation, including the
// root fence of every populated and empty result extent.
func (application Application) Available() bool {
	if !application.sealed || !application.dependency.Available() || !application.operation.Available() || !application.root.Available() {
		return false
	}
	return resultValidFor(application.root, application.operation, application.result)
}

// resultValidFor binds both populated and empty Apply extents to the exact
// mounted runtime fence carried by the observed root. Empty extents have no
// Application cell from which to recover that fence, so their authenticated
// scope tokens are checked directly.
func resultValidFor(root database.Version, operation signature.Identity, result apply.Results) bool {
	if !root.Available() || !operation.Available() || !result.Available() || result.Operation() != operation || !root.Fence().Available() {
		return false
	}
	fence := root.Fence()
	for _, scope := range result.Scopes() {
		if !scope.ValidFor(fence) {
			return false
		}
	}
	for index := 0; index < result.Len(); index++ {
		value, ok := result.At(index)
		if !ok || !value.Available() || value.Operation() != operation || !value.Fence().Same(fence) || !value.Invocation().ValidFor(fence) {
			return false
		}
	}
	return true
}

// Dependency returns the owner-issued schedule declaration for this
// observation.
func (application Application) Dependency() model.DependencyID {
	if !application.Available() {
		return model.DependencyID{}
	}
	return application.dependency
}

// Operation returns the exact schema-sealed operation that produced this
// extent. It remains available for a valid zero-application extent.
func (application Application) Operation() signature.Identity {
	if !application.Available() {
		return signature.Identity{}
	}
	return application.operation
}

// Root returns the database root observed by this application. A later
// evaluation records the successor root, never its predecessor.
func (application Application) Root() database.Version {
	if !application.Available() {
		return database.Version{}
	}
	return application.root
}

// Result returns the one authenticated Apply extent retained for this exact
// (Dependency, Operation) key. The extent itself is immutable.
func (application Application) Result() apply.Results {
	if !application.Available() {
		return apply.Results{}
	}
	return application.result
}

// Catalog is an immutable observation directory in the exact order supplied
// by mounted schema ObservationContracts. It deliberately uses bounded
// linear lookup: the sealed observation catalogue is small, and a second map
// would be duplicate mutable-looking authority rather than semantic state.
type Catalog struct {
	slots  []catalogSlot
	count  int
	sealed bool
}

type catalogSlot struct {
	dependency model.DependencyID
	operation  signature.Identity
	value      Application
	present    bool
}

// NewCatalog admits only mounted schema observation contracts. The contract
// order is the sealed traversal order and is retained exactly. Multiple
// contracts may project different views of one unique Apply occurrence; those
// declarations canonicalize to one semantic (Dependency, Operation) slot.
// Different operations under one dependency receive independent slots.
func NewCatalog(contracts []algebra.ObservationContract) (Catalog, bool) {
	slots := make([]catalogSlot, 0, len(contracts))
	for _, contract := range contracts {
		if !contract.Available() {
			return Catalog{}, false
		}
		dependency, operation := contract.Dependency(), contract.Operation()
		if !dependency.Available() || !operation.Available() {
			return Catalog{}, false
		}
		duplicate := false
		for _, prior := range slots {
			if prior.dependency == dependency && prior.operation == operation {
				duplicate = true
				break
			}
		}
		if !duplicate {
			slots = append(slots, catalogSlot{dependency: dependency, operation: operation})
		}
	}
	value := Catalog{slots: slots, sealed: true}
	return value, value.Available()
}

// Available authenticates the sealed declaration keys and every retained
// application without exposing implementation slices for mutation.
func (catalog Catalog) Available() bool {
	if !catalog.sealed || catalog.slots == nil {
		return false
	}
	count := 0
	for index, slot := range catalog.slots {
		if !slot.dependency.Available() || !slot.operation.Available() {
			return false
		}
		for _, prior := range catalog.slots[:index] {
			if prior.dependency == slot.dependency && prior.operation == slot.operation {
				return false
			}
		}
		if !slot.present {
			continue
		}
		if !slot.value.Available() || slot.value.Dependency() != slot.dependency || slot.value.Operation() != slot.operation {
			return false
		}
		count++
	}
	return count == catalog.count
}

// Len reports the number of retained declared Apply extents, not the number
// of schema contracts and not the number of applications inside an extent.
func (catalog Catalog) Len() int {
	if !catalog.Available() {
		return 0
	}
	return catalog.count
}

// Lookup redeems one exact composite observation key. A missing value is
// ordinary for a relation-only evaluation or a declared operation whose
// latest evaluation carried no extent. An empty authenticated Apply extent is
// still a present value and is returned successfully.
func (catalog Catalog) Lookup(dependency model.DependencyID, operation signature.Identity) (Application, bool) {
	if !catalog.Available() || !dependency.Available() || !operation.Available() {
		return Application{}, false
	}
	for _, slot := range catalog.slots {
		if slot.dependency != dependency || slot.operation != operation || !slot.present {
			continue
		}
		value := slot.value
		return value, value.Available() && value.Dependency() == dependency && value.Operation() == operation
	}
	return Application{}, false
}

// Declared reports whether the exact composite key was admitted from the
// mounted observation catalogue. It is distinct from Lookup: a declared key
// may currently have no retained value, while an undeclared result must be
// ignored rather than treated as a foreign terminal application.
func (catalog Catalog) Declared(dependency model.DependencyID, operation signature.Identity) bool {
	if !catalog.Available() || !dependency.Available() || !operation.Available() {
		return false
	}
	return catalog.index(dependency, operation) >= 0
}

// CompleteDependency reports whether every observation contract admitted for
// one dependency has a retained extent. A declared Apply owns an extent even
// when that extent contains zero applications, so a missing declared key is a
// transport/evaluator failure rather than ordinary relation-only absence.
func (catalog Catalog) CompleteDependency(dependency model.DependencyID) bool {
	if !catalog.Available() || !dependency.Available() {
		return false
	}
	for _, slot := range catalog.slots {
		if slot.dependency == dependency && !slot.present {
			return false
		}
	}
	return true
}

// Applications returns retained observations in sealed contract order.
// Relation-only and currently absent declarations are omitted.
func (catalog Catalog) Applications() []Application {
	if !catalog.Available() {
		return nil
	}
	result := make([]Application, 0, catalog.count)
	for _, slot := range catalog.slots {
		if slot.present {
			result = append(result, slot.value)
		}
	}
	return result
}

// Replace atomically replaces the observation for its exact sealed composite
// key. A foreign dependency or operation is refused.
func (catalog Catalog) Replace(application Application) (Catalog, bool) {
	if !catalog.Available() || !application.Available() {
		return Catalog{}, false
	}
	index := catalog.index(application.Dependency(), application.Operation())
	if index < 0 {
		return Catalog{}, false
	}
	result := catalog.clone()
	if !result.slots[index].present {
		result.count++
	}
	result.slots[index].value = application
	result.slots[index].present = true
	return result, result.Available()
}

// ClearDependency removes every retained operation declared for one
// dependency. It is called before recording a re-evaluation, so stale
// operations cannot survive when a later result carries only a subset of the
// dependency's declared Apply occurrences—or no application at all.
func (catalog Catalog) ClearDependency(dependency model.DependencyID) (Catalog, bool) {
	if !catalog.Available() || !dependency.Available() {
		return Catalog{}, false
	}
	needsCopy := false
	for _, slot := range catalog.slots {
		if slot.dependency == dependency && slot.present {
			needsCopy = true
			break
		}
	}
	if !needsCopy {
		return catalog, true
	}
	result := catalog.clone()
	for index := range result.slots {
		if result.slots[index].dependency != dependency || !result.slots[index].present {
			continue
		}
		result.slots[index].value = Application{}
		result.slots[index].present = false
		result.count--
	}
	return result, result.Available()
}

// index is intentionally bounded linear lookup over the sealed contract
// order. It is only called after Catalog and key availability checks.
func (catalog Catalog) index(dependency model.DependencyID, operation signature.Identity) int {
	for index, slot := range catalog.slots {
		if slot.dependency == dependency && slot.operation == operation {
			return index
		}
	}
	return -1
}

func (catalog Catalog) clone() Catalog {
	return Catalog{
		slots:  append([]catalogSlot{}, catalog.slots...),
		count:  catalog.count,
		sealed: catalog.sealed,
	}
}
