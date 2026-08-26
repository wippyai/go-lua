package authority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
)

// RelationSpec is the raw owner declaration of one relation. Token is the
// content identity issued by the owner for this relation. Column and key
// membership is derived once from the ordered ColumnSpec and KeySpec inputs;
// it is deliberately not repeated here.
type RelationSpec struct {
	Name        schema.Key
	Token       identity.ContentID
	Scope       schema.Key
	Addressing  []Address
	Publication schema.Key
}

// Available reports whether the relation's own fields are structurally
// present. Cross-row references are checked by NewCatalog.
func (spec RelationSpec) Available() bool {
	if !spec.Name.Available() || !spec.Token.Available() || !spec.Scope.Available() {
		return false
	}
	if spec.Publication != "" && !spec.Publication.Available() {
		return false
	}
	for _, address := range spec.Addressing {
		if !address.Available() {
			return false
		}
	}
	return true
}

// ColumnSpec is the raw owner declaration of one relation column. Type is
// already the complete owner-issued model.TypeID, including when its owner is
// a different sealed surface.
type ColumnSpec struct {
	Name     schema.Key
	Token    identity.ContentID
	Relation schema.Key
	Type     model.TypeID
}

// Available reports whether all column fields are structurally present.
func (spec ColumnSpec) Available() bool {
	return spec.Name.Available() && spec.Token.Available() && spec.Relation.Available() && spec.Type.Available()
}

// KeySpec is the raw owner declaration of one ordered key vector.
type KeySpec struct {
	Name     schema.Key
	Token    identity.ContentID
	Relation schema.Key
	Columns  []schema.Key
}

// Available reports whether the key's ordered vector is structurally valid.
// An empty vector is retained: the model and Registry represent an empty key
// as a valid logical vector, while NewCatalog still rejects malformed labels
// and duplicate members.
func (spec KeySpec) Available() bool {
	return spec.Name.Available() && spec.Token.Available() && spec.Relation.Available() && labelsAvailable(spec.Columns)
}

// ScopeSpec is the raw owner declaration of one decision scope. A scope may
// validly have no dimensions; that is the empty conjunction accepted by the
// model and by relcompile.Registry.
type ScopeSpec struct {
	Name       schema.Key
	Token      identity.ContentID
	Dimensions []schema.Key
	Region     region.Region
}

// Available reports whether the scope's own fields are structurally present.
func (spec ScopeSpec) Available() bool {
	return spec.Name.Available() && spec.Token.Available() && labelsAvailable(spec.Dimensions) && spec.Region.Available() && !spec.Region.IsFalse()
}

// DenominatorSpec is the raw owner declaration of one relation/key universe.
// A denominator has no independent model identity: model.DenominatorRef is
// the canonical pair of the owner-issued relation and key IDs.
type DenominatorSpec struct {
	Name     schema.Key
	Relation schema.Key
	Key      schema.Key
}

// Available reports whether all local labels are present. Pair membership is
// checked by NewCatalog.
func (spec DenominatorSpec) Available() bool {
	return spec.Name.Available() && spec.Relation.Available() && spec.Key.Available()
}

func labelsAvailable(labels []schema.Key) bool {
	for _, label := range labels {
		if !label.Available() {
			return false
		}
	}
	return true
}

func cloneLabels(labels []schema.Key) []schema.Key {
	if labels == nil {
		return nil
	}
	return append([]schema.Key(nil), labels...)
}

func cloneAddresses(addresses []Address) []Address {
	if addresses == nil {
		return nil
	}
	return append([]Address(nil), addresses...)
}
