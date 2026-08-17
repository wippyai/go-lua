package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

func (decoder *staticArtifactDecoder) references(output *ReferencesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightReferences(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactReferenceWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeRef = make([]TypeRef, count)
	}
	for index := 0; index < count; index++ {
		resolution, err := decoder.enum(uint64(TypeRefCanonicalPath))
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		root, err := decoder.term()
		if err != nil {
			return err
		}
		sourceKeys, err := decoder.keys()
		if err != nil {
			return err
		}
		sourceKeyCount := decoder.lastKeyCount
		canonicalKeys, err := decoder.keysAllowEmpty()
		if err != nil {
			return err
		}
		canonicalKeyCount := decoder.lastKeyCount
		switch TypeRefResolution(resolution) {
		case TypeRefUnresolved:
			if target != 0 || canonicalKeyCount != 0 {
				return errInvalidArtifactSection
			}
		case TypeRefDeclaration:
			if !staticrole.TypeReferenceTargetFamily(keyspace.TermFamily(target)) || canonicalKeyCount != 0 {
				return errInvalidArtifactSection
			}
		case TypeRefCanonicalPath:
			if target != 0 || canonicalKeyCount == 0 {
				return errInvalidArtifactSection
			}
		default:
			return errInvalidArtifactSection
		}
		if sourceKeyCount == 1 && root != 0 {
			return errInvalidArtifactSection
		}
		if sourceKeyCount > 1 && keyspace.TermFamily(root) != keyspace.FamilyCell {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeRef[index] = TypeRef{
				Resolution: TypeRefResolution(resolution),
				Target:     target,
				Root:       root,
				Source:     sourceKeys,
				Canonical:  canonicalKeys,
			}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) keys() ([]keyspace.Key, error) {
	return decoder.keysWithMinimum(1)
}

func (decoder *staticArtifactDecoder) keysAllowEmpty() ([]keyspace.Key, error) {
	return decoder.keysWithMinimum(0)
}

func (decoder *staticArtifactDecoder) keysWithMinimum(minimum int) ([]keyspace.Key, error) {
	if !decoder.probing && !decoder.preflighted {
		probe, err := decoder.probeReader()
		if err != nil {
			return nil, err
		}
		if _, err := probe.keysWithMinimum(minimum); err != nil {
			return nil, err
		}
	}
	count, err := decoder.count(staticArtifactUintWireMin)
	if err != nil {
		return nil, err
	}
	if count < minimum {
		return nil, errInvalidArtifactSection
	}
	decoder.lastKeyCount = count
	var keys []keyspace.Key
	if !decoder.probing {
		keys = make([]keyspace.Key, count)
	}
	for index := 0; index < count; index++ {
		key, readErr := decoder.key()
		if readErr != nil {
			return nil, readErr
		}
		if !decoder.probing {
			keys[index] = key
		}
	}
	return keys, nil
}
