// Package targetfixture is a test-only mount substrate for target relation
// runtime specimens. It deliberately owns no domain codecs, values, or rules.
package targetfixture

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Probe is the small test failure surface used by fixture families.
type Probe interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
}

// Identity issues one fixture owner's logical vocabulary from a fixture-local
// namespace. Every relation, column, key, scope, row, and operation remains
// owner-issued; no helper derives identity from a physical address or ordinal.
type Identity struct {
	domain string
	owner  model.OwnerID
}

// NewIdentity creates the one owner used by a test specimen.
func NewIdentity(t Probe, domain string) Identity {
	t.Helper()
	content, ok := identity.DeriveContentID("analysis/engine/relation/runtime/testdata/targetfixture/owner/v1", []byte(domain))
	if !ok {
		t.Fatalf("target fixture owner content for %q", domain)
	}
	owner, ok := model.IssueOwnerID(content)
	if !ok {
		t.Fatalf("target fixture owner for %q", domain)
	}
	return Identity{domain: domain, owner: owner}
}

// Domain returns the domain separation used by this fixture's identity owner.
func (value Identity) Domain() string { return value.domain }

// Owner returns the fixture's one logical identity issuer.
func (value Identity) Owner() model.OwnerID { return value.owner }

// Content derives fixture-local logical content. It is intentionally exposed
// so families can issue related RowIDs from one shared logical row content.
func (value Identity) Content(label string) (identity.ContentID, bool) {
	if !value.owner.Available() || value.domain == "" || label == "" {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/engine/relation/runtime/testdata/targetfixture/content/v1", []byte(value.domain), []byte(label))
}

func (value Identity) mustContent(t Probe, label string) identity.ContentID {
	t.Helper()
	content, ok := value.Content(label)
	if !ok {
		t.Fatalf("target fixture content %q", label)
	}
	return content
}

// Schema issues one schema identity under this fixture owner.
func (value Identity) Schema(t Probe, label string) model.SchemaID {
	t.Helper()
	result, ok := model.IssueSchemaID(value.owner, value.mustContent(t, "schema/"+label))
	if !ok {
		t.Fatalf("target fixture schema %q", label)
	}
	return result
}

// Type issues one nominal semantic type identity.
func (value Identity) Type(t Probe, label string) model.TypeID {
	t.Helper()
	result, ok := model.IssueTypeID(value.owner, value.mustContent(t, "type/"+label))
	if !ok {
		t.Fatalf("target fixture type %q", label)
	}
	return result
}

// Scope issues one decision-scope identity.
func (value Identity) Scope(t Probe, label string) model.ScopeID {
	t.Helper()
	result, ok := model.IssueScopeID(value.owner, value.mustContent(t, "scope/"+label))
	if !ok {
		t.Fatalf("target fixture scope %q", label)
	}
	return result
}

// Relation issues one nominal relation identity.
func (value Identity) Relation(t Probe, label string) model.RelationID {
	t.Helper()
	result, ok := model.IssueRelationID(value.owner, value.mustContent(t, "relation/"+label))
	if !ok {
		t.Fatalf("target fixture relation %q", label)
	}
	return result
}

// Column issues one column under relation.
func (value Identity) Column(t Probe, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	result, ok := model.IssueColumnID(relation, value.mustContent(t, fmt.Sprintf("column/%x/%s", relation.Content(), label)))
	if !ok {
		t.Fatalf("target fixture column %q", label)
	}
	return result
}

// Key issues one relation-local key identity.
func (value Identity) Key(t Probe, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	result, ok := model.IssueKeyID(relation, value.mustContent(t, fmt.Sprintf("key/%x/%s", relation.Content(), label)))
	if !ok {
		t.Fatalf("target fixture key %q", label)
	}
	return result
}

// Row issues one relation-local row from caller-selected logical content.
func (value Identity) Row(t Probe, relation model.RelationID, label string) model.RowID {
	t.Helper()
	result, ok := model.IssueRowID(relation, value.mustContent(t, "row/"+label))
	if !ok {
		t.Fatalf("target fixture row %q", label)
	}
	return result
}

// RowFromContent issues one relation-local row from shared logical content.
func (value Identity) RowFromContent(t Probe, relation model.RelationID, content identity.ContentID) model.RowID {
	t.Helper()
	result, ok := model.IssueRowID(relation, content)
	if !ok {
		t.Fatal("target fixture row from content")
	}
	return result
}

// Operation issues one semantic operation identity.
func (value Identity) Operation(t Probe, label string) model.OperationID {
	t.Helper()
	result, ok := model.IssueOperationID(value.owner, value.mustContent(t, "operation/"+label))
	if !ok {
		t.Fatalf("target fixture operation %q", label)
	}
	return result
}

// Expression issues one logical expression identity.
func (value Identity) Expression(t Probe, label string) model.ExpressionID {
	t.Helper()
	result, ok := model.IssueExpressionID(value.owner, value.mustContent(t, "expression/"+label))
	if !ok {
		t.Fatalf("target fixture expression %q", label)
	}
	return result
}

// Dependency issues one logical dependency identity.
func (value Identity) Dependency(t Probe, label string) model.DependencyID {
	t.Helper()
	result, ok := model.IssueDependencyID(value.owner, value.mustContent(t, "dependency/"+label))
	if !ok {
		t.Fatalf("target fixture dependency %q", label)
	}
	return result
}

// Refusal issues one semantic refusal identity.
func (value Identity) Refusal(t Probe, label string) model.RefusalID {
	t.Helper()
	result, ok := model.IssueRefusalID(value.owner, value.mustContent(t, "refusal/"+label))
	if !ok {
		t.Fatalf("target fixture refusal %q", label)
	}
	return result
}
