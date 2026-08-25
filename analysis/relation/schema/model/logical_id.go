package model

import "github.com/wippyai/go-lua/analysis/identity"

// SchemaID names one owner-issued logical schema artifact.
type SchemaID struct {
	issued
}

// IssueSchemaID adopts a non-zero token issued by owner.
func IssueSchemaID(owner OwnerID, content identity.ContentID) (SchemaID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return SchemaID{}, false
	}
	return SchemaID{issued: value}, true
}

// Available reports whether id names an issued schema artifact.
func (id SchemaID) Available() bool { return id.issued.Available() }

// Owner returns the authority that issued id.
func (id SchemaID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued schema token.
func (id SchemaID) Content() identity.ContentID { return id.issued.content }

// ExpressionID names one owner-issued logical expression.
type ExpressionID struct {
	issued
}

// IssueExpressionID adopts a non-zero token issued by owner.
func IssueExpressionID(owner OwnerID, content identity.ContentID) (ExpressionID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return ExpressionID{}, false
	}
	return ExpressionID{issued: value}, true
}

// Available reports whether id names an issued expression.
func (id ExpressionID) Available() bool { return id.issued.Available() }

// Owner returns the authority that issued id.
func (id ExpressionID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued expression token.
func (id ExpressionID) Content() identity.ContentID { return id.issued.content }

// DependencyID names one owner-issued logical dependency.
type DependencyID struct {
	issued
}

// IssueDependencyID adopts a non-zero token issued by owner.
func IssueDependencyID(owner OwnerID, content identity.ContentID) (DependencyID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return DependencyID{}, false
	}
	return DependencyID{issued: value}, true
}

// Available reports whether id names an issued dependency.
func (id DependencyID) Available() bool { return id.issued.Available() }

// Owner returns the authority that issued id.
func (id DependencyID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued dependency token.
func (id DependencyID) Content() identity.ContentID { return id.issued.content }

// OperationID names one owner-issued semantic operation.
type OperationID struct {
	issued
}

// IssueOperationID adopts a non-zero token issued by owner.
func IssueOperationID(owner OwnerID, content identity.ContentID) (OperationID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return OperationID{}, false
	}
	return OperationID{issued: value}, true
}

// Available reports whether id names an issued operation.
func (id OperationID) Available() bool { return id.issued.Available() }

// Owner returns the authority that issued id.
func (id OperationID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued operation token.
func (id OperationID) Content() identity.ContentID { return id.issued.content }

// TypeID names one owner-issued semantic type.
type TypeID struct {
	issued
}

// IssueTypeID adopts a non-zero token issued by owner.
func IssueTypeID(owner OwnerID, content identity.ContentID) (TypeID, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return TypeID{}, false
	}
	return TypeID{issued: value}, true
}

// Available reports whether id names an issued semantic type.
func (id TypeID) Available() bool { return id.issued.Available() }

// Owner returns the authority that issued id.
func (id TypeID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued type token.
func (id TypeID) Content() identity.ContentID { return id.issued.content }

// RowID names one relation-owned logical key value. The relation fence keeps
// equal content tokens in different relations nominally distinct.
type RowID struct {
	relation RelationID
	issued
}

// IssueRowID adopts a non-zero token issued in relation's authority.
func IssueRowID(relation RelationID, content identity.ContentID) (RowID, bool) {
	value, ok := issue(relation.Owner(), content)
	if !relation.Available() || !ok {
		return RowID{}, false
	}
	return RowID{relation: relation, issued: value}, true
}

// Available reports whether id names an issued row value.
func (id RowID) Available() bool {
	return id.relation.Available() && id.issued.Available()
}

// Relation returns the exact relation that issued id.
func (id RowID) Relation() RelationID { return id.relation }

// Owner returns the authority that issued id through its relation.
func (id RowID) Owner() OwnerID { return id.issued.owner }

// Content returns the owner-issued row token.
func (id RowID) Content() identity.ContentID { return id.issued.content }
