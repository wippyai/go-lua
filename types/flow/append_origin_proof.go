package flow

import "github.com/wippyai/go-lua/types/constraint"

// AppendOriginDestination is a routed destination for appended element-field
// origins. FieldPrefix is the suffix already accumulated when the append is
// reached through an iterator or assignment alias.
type AppendOriginDestination struct {
	Array       StableAddress
	FieldPrefix []constraint.Segment
}

// AppendOriginSource is a routed source for one appended field origin.
// SourceField is non-empty when the source path denotes the containing element
// and the field suffix is relative to that element.
type AppendOriginSource struct {
	Source      StableAddress
	SourceField []constraint.Segment
}

// AppendOriginDestinations follows value-origin and path-alias facts to every
// array whose appended element field may be observed through array.
func AppendOriginDestinations(state PointState, array StableAddress, fieldPrefix []constraint.Segment) []AppendOriginDestination {
	if array.Key() == "" {
		return nil
	}
	seen := map[string]bool{}
	var destinations []AppendOriginDestination
	var add func(StableAddress, []constraint.Segment)
	add = func(array StableAddress, prefix []constraint.Segment) {
		key := array.Key()
		seenKey := string(key) + "/" + string(AppendElementFieldPathKey(prefix))
		if key == "" || seen[seenKey] {
			return
		}
		seen[seenKey] = true
		destinations = append(destinations, AppendOriginDestination{
			Array:       array,
			FieldPrefix: cloneAddressSegments(prefix),
		})
		for _, use := range state.ValueOrigins.OriginsCoveringAddress(array) {
			if use.Origin.Kind != ValueOriginIndexedIterator || use.Origin.VarIndex != 1 || len(use.Remainder) == 0 {
				continue
			}
			source, ok := StableAddressFromKey(use.Origin.Source)
			if !ok {
				continue
			}
			nextPrefix := cloneAddressSegments(use.Remainder)
			nextPrefix = append(nextPrefix, prefix...)
			add(source, nextPrefix)
		}
		for _, aliasUse := range state.PathAliases.AliasesCoveringAddress(array) {
			source, ok := StableAddressFromKey(aliasUse.Alias.Source)
			if !ok {
				continue
			}
			source, ok = source.Append(aliasUse.Remainder)
			if ok {
				add(source, prefix)
			}
		}
	}
	add(array, fieldPrefix)
	return destinations
}

// AppendOriginSources follows value-origin and path-alias facts backward from
// source to every path that may have supplied an appended element field.
func AppendOriginSources(state PointState, source StableAddress) []AppendOriginSource {
	if source.Key() == "" {
		return nil
	}
	var sources []AppendOriginSource
	add := func(source StableAddress, sourceField []constraint.Segment) {
		if source.Key() == "" {
			return
		}
		sources = append(sources, AppendOriginSource{
			Source:      source,
			SourceField: cloneAddressSegments(sourceField),
		})
	}
	add(source, nil)
	for _, use := range state.ValueOrigins.OriginsCoveringAddress(source) {
		routed, ok := StableAddressFromKey(use.Origin.Source)
		if !ok {
			continue
		}
		switch use.Origin.Kind {
		case ValueOriginIndexedIterator:
			if use.Origin.VarIndex == 1 && len(use.Remainder) > 0 {
				add(routed, use.Remainder)
			}
		case ValueOriginAssignmentAlias:
			routed, ok = routed.Append(use.Remainder)
			if ok {
				add(routed, nil)
			}
		}
	}
	for _, aliasUse := range state.PathAliases.AliasesCoveringAddress(source) {
		routed, ok := StableAddressFromKey(aliasUse.Alias.Source)
		if !ok {
			continue
		}
		routed, ok = routed.Append(aliasUse.Remainder)
		if ok {
			add(routed, nil)
		}
	}
	return sources
}

// ApplyAppendElementFieldOriginUse replays a prior field-origin use into the
// current append destinations.
func ApplyAppendElementFieldOriginUse(
	out *PointState,
	destinations []AppendOriginDestination,
	field []constraint.Segment,
	originUse ValueOriginUse,
) bool {
	if out == nil {
		return false
	}
	if originUse.Origin.Kind != ValueOriginIndexedIterator || originUse.Origin.VarIndex != 1 || len(originUse.Remainder) == 0 {
		return false
	}
	before := out.KeyPresence
	for _, sourceUse := range out.KeyPresence.AppendElementFieldSources(originUse.Origin.Source, originUse.Remainder) {
		source, ok := StableAddressFromKey(sourceUse.Origin.Source)
		if !ok {
			continue
		}
		sourceField := cloneAddressSegments(sourceUse.SourceField)
		if len(sourceField) > 0 {
			sourceField = append(sourceField, sourceUse.FieldRemainder...)
		} else {
			source, ok = source.Append(sourceUse.FieldRemainder)
			if !ok {
				continue
			}
		}
		for _, dst := range destinations {
			dstField := cloneAddressSegments(dst.FieldPrefix)
			dstField = append(dstField, field...)
			ApplyAppendElementFieldOriginProof(out, AppendElementFieldOriginProof{
				Array:       dst.Array,
				Field:       dstField,
				Source:      source,
				SourceField: sourceField,
			})
		}
	}
	return !KeyPresenceFactsDomain.Equal(before, out.KeyPresence)
}
