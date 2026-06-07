package flow

import "github.com/wippyai/go-lua/types/constraint"

// ProvenanceRoutes returns one-step source routes that may explain a read of
// path. It composes the currently separate alias, value-origin, and append-field
// origin domains into one normalized route boundary for checker layers.
func (f PointFacts) ProvenanceRoutes(path constraint.Path) []ProvenanceRoute {
	target, ok := StableAddressOfPath(path)
	if !ok {
		return nil
	}
	var out []ProvenanceRoute
	for _, source := range IdentityAliasSourcesWithPolicy(f.state, target, IdentityAliasReadPolicy) {
		sourcePath, ok := source.Path()
		if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
			continue
		}
		out = append(out, ProvenanceRoute{
			Kind:   ProvenanceRouteIdentityAlias,
			Source: sourcePath,
		})
	}
	for _, use := range f.state.ValueOrigins.OriginsCoveringAddress(target) {
		switch use.Origin.Kind {
		case ValueOriginIndexedIterator:
			out = appendAppendFieldProvenanceRoutes(out, f.state, use)
			out = appendValueOriginProvenanceRoute(out, use, ProvenanceRouteIndexedIterator)
		case ValueOriginKeyedIterator:
			out = appendValueOriginProvenanceRoute(out, use, ProvenanceRouteKeyedIterator)
		}
	}
	return out
}

func appendValueOriginProvenanceRoute(out []ProvenanceRoute, use ValueOriginUse, kind ProvenanceRouteKind) []ProvenanceRoute {
	source, ok := use.Origin.SourcePath()
	if !ok || source.IsEmpty() || source.Symbol == 0 {
		return out
	}
	return append(out, ProvenanceRoute{
		Kind:      kind,
		Source:    source,
		Remainder: cloneAddressSegments(use.Remainder),
		VarIndex:  use.Origin.VarIndex,
	})
}

func appendAppendFieldProvenanceRoutes(out []ProvenanceRoute, state PointState, use ValueOriginUse) []ProvenanceRoute {
	if use.Origin.Source == "" || len(use.Remainder) == 0 {
		return out
	}
	for _, fieldUse := range state.KeyPresence.AppendElementFieldSources(use.Origin.Source, use.Remainder) {
		source, ok := fieldUse.SourcePath()
		if !ok || source.IsEmpty() || source.Symbol == 0 {
			continue
		}
		out = append(out, ProvenanceRoute{
			Kind:           ProvenanceRouteAppendElementField,
			Source:         source,
			SourceField:    cloneAddressSegments(fieldUse.SourceField),
			FieldRemainder: cloneAddressSegments(fieldUse.FieldRemainder),
		})
	}
	return out
}
