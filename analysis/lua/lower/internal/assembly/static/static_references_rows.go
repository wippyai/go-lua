package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
)

// TypeRefUnresolved/Declaration/Canonical preserve both the source spelling
// and the exact binder disposition. The rows retain raw path payloads only;
// the static construction methods coordinate each path atom with Source
// before it invokes these pure row operations.
func (rows *staticRows) TypeRefUnresolved(term, root keyspace.Term, path []string) error {
	raw, err := staticRawPath(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, staticrefs.Unresolved, 0, root, raw, nil)
}

func (rows *staticRows) TypeRefDeclaration(term, root, target keyspace.Term, path []string) error {
	raw, err := staticRawPath(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, staticrefs.Declaration, target, root, raw, nil)
}

func (rows *staticRows) TypeRefCanonical(term, root keyspace.Term, path, canonical []string) error {
	raw, err := staticRawPath(path)
	if err != nil {
		return err
	}
	resolution, err := staticRawPath(canonical)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, staticrefs.CanonicalPath, 0, root, raw, resolution)
}

func (rows *staticRows) typeRefRaw(term keyspace.Term, resolution staticrefs.Resolution, target, root keyspace.Term, path, canonical []staticRawKey) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyTypeRef || keyspace.TermOrdinal(term) != uint32(len(rows.references)+1) {
		return errors.New("program/lower/collector: invalid TypeRef term")
	}
	if len(path) == 0 || (resolution == staticrefs.CanonicalPath && len(canonical) == 0) || (resolution != staticrefs.CanonicalPath && len(canonical) != 0) {
		return errors.New("program/lower/collector: invalid TypeRef path disposition")
	}
	if resolution == staticrefs.Declaration && target == 0 || resolution != staticrefs.Declaration && target != 0 {
		return errors.New("program/lower/collector: invalid TypeRef target disposition")
	}
	rows.references = append(rows.references, staticRawTypeRef{resolution: resolution, target: target, root: root, source: append([]staticRawKey(nil), path...), canonical: append([]staticRawKey(nil), canonical...)})
	return nil
}
