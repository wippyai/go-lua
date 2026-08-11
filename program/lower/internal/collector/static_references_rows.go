package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// TypeRefUnresolved/Declaration/Canonical preserve both the source spelling
// and the exact binder disposition. The rows retain raw path payloads only;
// the public StaticReferences view coordinates each path atom with Source
// before it invokes these pure row operations.
func (rows *staticRows) TypeRefUnresolved(term, root keyspace.Term, path []string) error {
	raw, err := staticRawPath(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, programstatic.TypeRefUnresolved, 0, root, raw, nil)
}

func (rows *staticRows) TypeRefDeclaration(term, root, target keyspace.Term, path []string) error {
	raw, err := staticRawPath(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, programstatic.TypeRefDeclaration, target, root, raw, nil)
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
	return rows.typeRefRaw(term, programstatic.TypeRefCanonicalPath, 0, root, raw, resolution)
}

func (rows *staticRows) TypeRefUnresolvedRaw(term, root keyspace.Term, path []keyspace.LiteralValue) error {
	raw, err := rows.rawKeys(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, programstatic.TypeRefUnresolved, 0, root, raw, nil)
}

func (rows *staticRows) TypeRefDeclarationRaw(term, root, target keyspace.Term, path []keyspace.LiteralValue) error {
	raw, err := rows.rawKeys(path)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, programstatic.TypeRefDeclaration, target, root, raw, nil)
}

func (rows *staticRows) TypeRefCanonicalRaw(term, root keyspace.Term, path, canonical []keyspace.LiteralValue) error {
	raw, err := rows.rawKeys(path)
	if err != nil {
		return err
	}
	resolution, err := rows.rawKeys(canonical)
	if err != nil {
		return err
	}
	return rows.typeRefRaw(term, programstatic.TypeRefCanonicalPath, 0, root, raw, resolution)
}

func (rows *staticRows) rawKeys(values []keyspace.LiteralValue) ([]staticRawKey, error) {
	if len(values) == 0 {
		return nil, errors.New("program/lower/collector: empty Static type path")
	}
	result := make([]staticRawKey, len(values))
	for index, value := range values {
		if value.Kind != keyspace.LiteralString || value.String == "" {
			return nil, errors.New("program/lower/collector: Static type path component is not a non-empty string")
		}
		key, err := rawLiteral(value)
		if err != nil {
			return nil, err
		}
		result[index] = key
	}
	return result, nil
}

func (rows *staticRows) typeRefRaw(term keyspace.Term, resolution programstatic.TypeRefResolution, target, root keyspace.Term, path, canonical []staticRawKey) error {
	if rows == nil || term == 0 || keyspace.TermFamily(term) != keyspace.FamilyTypeRef || keyspace.TermOrdinal(term) != uint32(len(rows.references)+1) {
		return errors.New("program/lower/collector: invalid TypeRef term")
	}
	if len(path) == 0 || (resolution == programstatic.TypeRefCanonicalPath && len(canonical) == 0) || (resolution != programstatic.TypeRefCanonicalPath && len(canonical) != 0) {
		return errors.New("program/lower/collector: invalid TypeRef path disposition")
	}
	if resolution == programstatic.TypeRefDeclaration && target == 0 || resolution != programstatic.TypeRefDeclaration && target != 0 {
		return errors.New("program/lower/collector: invalid TypeRef target disposition")
	}
	rows.references = append(rows.references, staticRawTypeRef{resolution: resolution, target: target, root: root, source: append([]staticRawKey(nil), path...), canonical: append([]staticRawKey(nil), canonical...)})
	return nil
}
