// Package expandcatalog freezes owner evidence before arrangement lowering.
// It is a mount-admission child, not a second logical algebra vocabulary.
package expandcatalog

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	mountexpand "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Freeze walks checked expressions once and freezes every declared Expand
// vector under the mounted issuer. Pointer and value algebra forms normalize
// to the same digest, so evidence cannot be skipped by representation.
// resolve is used only during this mount admission call; it is never retained
// in the catalog or exposed to arrangement/runtime.
func Freeze(cert certificate.Certificate, resolve func(model.ExpandContract) ([]mountexpand.Vector, bool), issuer binding.Issuer, fence binding.Fence) (mountexpand.Catalog, bool) {
	if !cert.Available() || resolve == nil || !issuer.Available() || !fence.Available() {
		return mountexpand.Catalog{}, false
	}
	types := make(map[model.ColumnID]model.TypeID, len(cert.Columns()))
	for _, column := range cert.Columns() {
		if !column.Available() || !column.ID().Available() || !column.Type().Available() {
			return mountexpand.Catalog{}, false
		}
		if _, duplicate := types[column.ID()]; duplicate {
			return mountexpand.Catalog{}, false
		}
		types[column.ID()] = column.Type()
	}
	entries := make([]mountexpand.Entry, 0)
	seen := make(map[identity.ContentID]struct{})
	for _, reference := range cert.Expressions() {
		if !reference.Available() || reference.Expression() == nil || !collect(reference.Expression(), resolve, issuer, fence, types, seen, &entries) {
			return mountexpand.Catalog{}, false
		}
	}
	if len(entries) == 0 {
		return mountexpand.EmptyCatalog(), true
	}
	return mountexpand.NewCatalog(entries)
}

func collect(expression algebra.Expression, resolve func(model.ExpandContract) ([]mountexpand.Vector, bool), issuer binding.Issuer, fence binding.Fence, types map[model.ColumnID]model.TypeID, seen map[identity.ContentID]struct{}, entries *[]mountexpand.Entry) bool {
	expression, ok := normalize(expression)
	if !ok || !expression.Digest().Available() {
		return false
	}
	digest := expression.Digest()
	if _, visited := seen[digest]; visited {
		return true
	}
	seen[digest] = struct{}{}
	child := func(value algebra.Expression) bool {
		return collect(value, resolve, issuer, fence, types, seen, entries)
	}
	switch value := expression.(type) {
	case algebra.Input:
		return true
	case algebra.Select:
		return child(value.Child())
	case algebra.Project:
		return child(value.Child())
	case algebra.ColumnProject:
		return child(value.Child())
	case algebra.Join:
		return child(value.Left()) && child(value.Right())
	case algebra.Merge:
		for _, input := range value.Inputs() {
			if !child(input) {
				return false
			}
		}
		return true
	case algebra.Group:
		return child(value.Child())
	case algebra.Complete:
		return child(value.Child())
	case algebra.Apply:
		for _, input := range value.Inputs() {
			if !child(input) {
				return false
			}
		}
		return true
	case algebra.Publish:
		return child(value.Child())
	case algebra.Expand:
		if !child(value.Child()) || !value.Contract().Available() {
			return false
		}
		keyType, ok := types[value.Contract().Key()]
		if !ok {
			return false
		}
		raw, ok := resolve(value.Contract())
		if !ok || raw == nil {
			return false
		}
		evidence, ok := mountexpand.Freeze(fence, issuer, value.Contract(), keyType, raw)
		if !ok || !evidence.Available() {
			return false
		}
		*entries = append(*entries, mountexpand.Entry{Expression: digest, Evidence: evidence})
		return true
	default:
		return false
	}
}

func normalize(expression algebra.Expression) (algebra.Expression, bool) {
	switch value := expression.(type) {
	case *algebra.Input:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Select:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Project:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.ColumnProject:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Join:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Merge:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Group:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Complete:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Apply:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Publish:
		if value == nil {
			return nil, false
		}
		return *value, true
	case *algebra.Expand:
		if value == nil {
			return nil, false
		}
		return *value, true
	default:
		return expression, expression != nil
	}
}
