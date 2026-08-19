package semanticpath

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func deriveTermPaths(sourceView source.View, cellRoles source.CellRoles, view authored.View, bindings binding.Result, bodies *body.Result, forest *containment.Result, outcomes *outcome.Result, edges [keyspace.FamilyCount][]edgeDescriptor, bodyPaths []identity.ContentID, descriptors [keyspace.FamilyCount][]identity.ContentID, roots [keyspace.FamilyCount][]identity.ContentID, resolver *structuralResolver) ([keyspace.FamilyCount][]identity.ContentID, error) {
	var paths [keyspace.FamilyCount][]identity.ContentID
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		// Every family owns its ordinal-zero sentinel, including an empty
		// denominator. Certificate.Seal validates the same dense n+1 plane;
		// leaving an empty family nil would make a valid Program fail issuance
		// before any term-path derivation runs.
		paths[family] = make([]identity.ContentID, sourceView.Identity().FamilyCount(family)+1)
	}
	// Body has no enclosing containment parent in Source's term index. Its
	// canonical BodyPath is therefore the only lawful body-qualified TermPath
	// and is used for Body endpoint/Site identity without a synthetic root.
	for ordinal := 1; ordinal < len(paths[keyspace.FamilyBody]); ordinal++ {
		if ordinal > len(bodyPaths) || !bodyPaths[ordinal-1].Available() {
			return paths, errors.New("semanticpath: Body term path is unavailable")
		}
		paths[keyspace.FamilyBody][ordinal] = bodyPaths[ordinal-1]
	}
	resolveFamily := func(family keyspace.Family) error {
		for ordinal := 1; ordinal <= len(paths[family])-1; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			bodyTerm, _, _, positioned := sourceView.Index().Position(term)
			if !positioned {
				// Static expression rows are valid Flow terms but deliberately
				// have no Source coordinate. The resolver anchors their exact
				// containment chain at the owner-issued Static scope boundary.
				bodyTerm = 0
			}
			path, ok := resolver.resolve(term, bodyTerm)
			if !ok {
				root, _ := sourceView.Index().Root(term)
				parent, _ := forest.Parent(term)
				missing, missingParent := resolver.missingStructuralEdge(term, bodyTerm)
				return fmt.Errorf("semanticpath: term %v family=%d ordinal=%d root=%v parent=%v missing-child=%v missing-parent=%v has no structural path", term, family, ordinal, root, parent, missing, missingParent)
			}
			paths[family][ordinal] = path
		}
		return nil
	}
	// Definition hosts all precede Static families in the stable keyspace.
	// Resolve those hosts first, then issue Cell identities from their exact
	// Binding roles. Static descendants whose canonical parent is a Cell must
	// start from that Cell identity rather than invent a generic Cell edge.
	for family := keyspace.Family(1); family < keyspace.FamilyTypeAlias; family++ {
		if family == keyspace.FamilyBody || family == keyspace.FamilyCell || family == keyspace.FamilyOutcome {
			continue
		}
		if err := resolveFamily(family); err != nil {
			return paths, err
		}
	}
	if err := deriveCellTermPaths(sourceView, cellRoles, view, bindings, bodies, forest, bodyPaths, &paths); err != nil {
		return paths, err
	}
	for ordinal := 1; ordinal < len(paths[keyspace.FamilyCell]); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		path := paths[keyspace.FamilyCell][ordinal]
		if !path.Available() {
			return paths, errors.New("semanticpath: Cell path is unavailable after role column derivation")
		}
		resolver.memo[cell] = path
	}
	for family := keyspace.FamilyTypeAlias; family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		if err := resolveFamily(family); err != nil {
			return paths, err
		}
	}
	outcomeCount := sourceView.Identity().FamilyCount(keyspace.FamilyOutcome)
	if outcomeCount > 0 && outcomes.Count() != outcomeCount {
		return paths, errors.New("semanticpath: Outcome denominator disagrees with Source")
	}
	for ordinal := 1; ordinal <= outcomeCount; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyOutcome, uint32(ordinal))
		bodyTerm, outcomeKind, target, ok := outcomes.Get(term)
		if !ok {
			return paths, fmt.Errorf("semanticpath: Outcome %v is unavailable", term)
		}
		bo := keyspace.TermOrdinal(bodyTerm)
		if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || bo == 0 || uint64(bo) > uint64(len(bodyPaths)) {
			return paths, errors.New("semanticpath: Outcome Body is invalid")
		}
		bodyPath := bodyPaths[bo-1]
		if !bodyPath.Available() {
			return paths, errors.New("semanticpath: Outcome Body path is unavailable")
		}
		var targetPath identity.ContentID
		if target != 0 {
			tf, to := keyspace.TermFamily(target), keyspace.TermOrdinal(target)
			if tf <= keyspace.FamilyInvalid || tf >= keyspace.FamilyCount || to == 0 || uint64(to) >= uint64(len(paths[tf])) {
				return paths, errors.New("semanticpath: Outcome target is invalid")
			}
			targetPath = paths[tf][to]
			if !targetPath.Available() {
				return paths, errors.New("semanticpath: Outcome target path is unavailable")
			}
		}
		paths[keyspace.FamilyOutcome][ordinal] = digestOutcome(bodyPath, uint32(outcomeKind), targetPath)
	}
	return paths, nil
}
