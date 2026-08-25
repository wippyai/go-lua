package typing

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// validateDeclarations proves typed row ownership after the shared registry
// has indexed every declaration. It intentionally owns shape/membership
// obligations; identity, availability, and digest findings come from the
// shared index and dependency projections belong to recurrence.
func (checker *checker) validateDeclarations() {
	for _, relation := range checker.registry.Relations() {
		if !relation.Available() {
			continue
		}
		path := relationPath(relation.ID())
		if scope := relation.Scope(); !scope.Available() {
			checker.report.add(CodeScopeMismatch, path, "relation has no decision scope")
		} else if _, ok := checker.registry.Scope(scope); !ok {
			checker.report.add(CodeMissingReference, path, "relation scope is not registered")
		}
		seenColumns := make(map[model.ColumnID]struct{})
		for _, id := range relation.Columns() {
			if _, duplicate := seenColumns[id]; duplicate {
				checker.report.add(CodeDuplicateMember, path, "relation repeats a column")
				continue
			}
			seenColumns[id] = struct{}{}
			column, ok := checker.registry.Column(id)
			if !ok {
				checker.report.add(CodeMissingReference, path, "relation column is not registered")
				continue
			}
			if column.Relation() != relation.ID() {
				checker.report.add(CodeMembership, path, "relation contains a column owned by another relation")
			}
		}
		seenKeys := make(map[model.KeyID]struct{})
		for _, id := range relation.Keys() {
			if _, duplicate := seenKeys[id]; duplicate {
				checker.report.add(CodeDuplicateMember, path, "relation repeats a key")
				continue
			}
			seenKeys[id] = struct{}{}
			key, ok := checker.registry.Key(id)
			if !ok {
				checker.report.add(CodeMissingReference, path, "relation key is not registered")
				continue
			}
			if key.Relation() != relation.ID() {
				checker.report.add(CodeMembership, path, "relation contains a key owned by another relation")
			}
		}
	}
	for _, key := range checker.registry.Keys() {
		if !key.Available() {
			continue
		}
		path := keyPath(key.ID())
		columns := key.Columns()
		if len(columns) == 0 {
			checker.report.add(CodeKeyMismatch, path, "key vector is empty")
		}
		seen := make(map[model.ColumnID]struct{}, len(columns))
		for _, id := range columns {
			if _, duplicate := seen[id]; duplicate {
				checker.report.add(CodeDuplicateMember, path, "key vector repeats a column")
				continue
			}
			seen[id] = struct{}{}
			column, ok := checker.registry.Column(id)
			if !ok {
				checker.report.add(CodeMissingReference, path, "key column is not registered")
				continue
			}
			if column.Relation() != key.Relation() {
				checker.report.add(CodeMembership, path, "key column belongs to another relation")
			}
		}
	}
	for _, scope := range checker.registry.Scopes() {
		if !scope.Available() {
			continue
		}
		path := scopePath(scope.ID())
		seen := make(map[model.ColumnID]struct{})
		for _, id := range scope.Dimensions() {
			if _, duplicate := seen[id]; duplicate {
				checker.report.add(CodeDuplicateMember, path, "scope repeats a dimension")
				continue
			}
			seen[id] = struct{}{}
			column, ok := checker.registry.Column(id)
			if !ok {
				checker.report.add(CodeMissingReference, path, "scope dimension is not registered")
				continue
			}
			if column.ID().Owner() != scope.Owner() {
				checker.report.add(CodeForeignReference, path, "scope dimension has a foreign owner")
			}
		}
	}
}
