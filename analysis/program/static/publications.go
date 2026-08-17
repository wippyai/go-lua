package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func compactPublications(component *Component, counts [keyspace.FamilyCount]uint32, input PublicationsInput) error {
	seen := make(map[publicationSlot]struct{}, len(input.Type))
	for _, row := range input.Type {
		if !hasFamily(counts, row.Assign, keyspace.FamilyAssign) ||
			!resolvedPublicationTarget(component, counts, row.Target) {
			return errors.New("program/static: invalid type publication")
		}
		slot := publicationSlot{assign: row.Assign, pair: row.Pair}
		if _, duplicate := seen[slot]; duplicate {
			return errors.New("program/static: duplicate type publication pair")
		}
		seen[slot] = struct{}{}
		component.publications = append(component.publications, publicationRow{
			assign: row.Assign,
			pair:   row.Pair,
			target: row.Target,
		})
	}
	return nil
}

// resolvedPublicationTarget accepts only the binder dispositions whose target
// is meaningful beyond the local spelling. Unresolved names are never made
// public merely because an Assign happens to contain one.
func resolvedPublicationTarget(component *Component, counts [keyspace.FamilyCount]uint32, target keyspace.Term) bool {
	if component == nil || !hasFamily(counts, target, keyspace.FamilyTypeRef) {
		return false
	}
	row := component.references.rows[keyspace.TermOrdinal(target)-1]
	return row.resolution == TypeRefDeclaration || row.resolution == TypeRefCanonicalPath
}

// emitPublicationsContainment contributes the resolved TypeRef child of each
// authored publication. Assign remains a Flow anchor and is intentionally not
// reconstructed or traversed here.
func emitPublicationsContainment(component *Component, check *containment) bool {
	for index, row := range component.publications {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(index+1)), row.target) {
			return false
		}
	}
	return true
}

// writePublicationsContent owns the exact authored Assign-pair-to-TypeRef
// relation. Duplicate-detection state and any future export projection are
// deliberately absent.
func writePublicationsContent(writer *framing.Writer, rows []publicationRow) error {
	if err := writer.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Uint(uint64(row.assign)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.pair)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
	}
	return nil
}
