package address

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Book is an immutable collection of typed, generation-fenced addresses for
// one checked certificate.  Its data pointer is private and there are no
// mutation methods; every enumeration returns a defensive copy.
type Book struct{ data *bookData }

type bookData struct {
	fence        Fence
	digest       identity.ContentID
	relations    map[model.RelationID]Address[model.RelationID]
	columns      map[model.ColumnID]Address[model.ColumnID]
	keys         map[model.KeyID]Address[model.KeyID]
	scopes       map[model.ScopeID]Address[model.ScopeID]
	expressions  map[model.ExpressionID]Address[model.ExpressionID]
	dependencies map[model.DependencyID]Address[model.DependencyID]

	relationIDs   []model.RelationID
	columnIDs     []model.ColumnID
	keyIDs        []model.KeyID
	scopeIDs      []model.ScopeID
	expressionIDs []model.ExpressionID
	dependencyIDs []model.DependencyID
}

// Available reports whether the book is a complete immutable mount binding.
func (book Book) Available() bool {
	return book.data != nil && book.data.fence.Available() && book.data.digest.Available()
}

// Fence returns the exact certificate/runtime fence captured at Bind time.
func (book Book) Fence() Fence {
	if book.data == nil {
		return Fence{}
	}
	return book.data.fence
}

// Digest returns the deterministic physical-book digest.  It includes the
// certificate digest, fence, and logical-to-local mapping; it is distinct
// from the certificate's logical identity.
func (book Book) Digest() identity.ContentID {
	if book.data == nil {
		return identity.ContentID{}
	}
	return book.data.digest
}

// Same reports whether two books describe the same complete mounted mapping.
func (book Book) Same(other Book) bool {
	return book.Available() && other.Available() && book.data.fence == other.data.fence && book.data.digest == other.data.digest
}

// Relation resolves one logical relation to its typed address.
func (book Book) Relation(id model.RelationID) (Address[model.RelationID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.RelationID]{}, false
	}
	value, ok := book.data.relations[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// Column resolves one logical column to its typed address.
func (book Book) Column(id model.ColumnID) (Address[model.ColumnID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.ColumnID]{}, false
	}
	value, ok := book.data.columns[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// Key resolves one logical key to its typed address.
func (book Book) Key(id model.KeyID) (Address[model.KeyID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.KeyID]{}, false
	}
	value, ok := book.data.keys[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// Scope resolves one logical scope to its typed address.
func (book Book) Scope(id model.ScopeID) (Address[model.ScopeID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.ScopeID]{}, false
	}
	value, ok := book.data.scopes[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// Expression resolves one logical expression to its typed address.
func (book Book) Expression(id model.ExpressionID) (Address[model.ExpressionID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.ExpressionID]{}, false
	}
	value, ok := book.data.expressions[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// Dependency resolves one logical dependency to its typed address.
func (book Book) Dependency(id model.DependencyID) (Address[model.DependencyID], bool) {
	if !book.Available() || !id.Available() {
		return Address[model.DependencyID]{}, false
	}
	value, ok := book.data.dependencies[id]
	return value, ok && value.ValidFor(book.data.fence)
}

// RelationIDs returns a deterministic defensive copy of the certified
// relation identities.
func (book Book) RelationIDs() []model.RelationID {
	if !book.Available() {
		return nil
	}
	return append([]model.RelationID(nil), book.data.relationIDs...)
}

// ColumnIDs returns a deterministic defensive copy of the certified column
// identities.
func (book Book) ColumnIDs() []model.ColumnID {
	if !book.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), book.data.columnIDs...)
}

// KeyIDs returns a deterministic defensive copy of the certified key
// identities.
func (book Book) KeyIDs() []model.KeyID {
	if !book.Available() {
		return nil
	}
	return append([]model.KeyID(nil), book.data.keyIDs...)
}

// ScopeIDs returns a deterministic defensive copy of the certified scope
// identities.
func (book Book) ScopeIDs() []model.ScopeID {
	if !book.Available() {
		return nil
	}
	return append([]model.ScopeID(nil), book.data.scopeIDs...)
}

// ExpressionIDs returns a deterministic defensive copy of the certified
// expression identities.
func (book Book) ExpressionIDs() []model.ExpressionID {
	if !book.Available() {
		return nil
	}
	return append([]model.ExpressionID(nil), book.data.expressionIDs...)
}

// DependencyIDs returns a deterministic defensive copy of the certified
// dependency identities.
func (book Book) DependencyIDs() []model.DependencyID {
	if !book.Available() {
		return nil
	}
	return append([]model.DependencyID(nil), book.data.dependencyIDs...)
}
