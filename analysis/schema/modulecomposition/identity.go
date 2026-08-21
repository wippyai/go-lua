package modulecomposition

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func resolvedImportID(row ResolvedImport) identity.ContentID {
	if !row.link.Available() || !row.sourceModuleKey.Available() || !row.targetModuleKey.Available() ||
		!row.artifactID.Available() || !row.programID.Available() || !row.importID.Available() || !row.requestID.Available() ||
		!row.valueID.Available() || row.requestKey == 0 {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/resolved-import/v1",
		idPart(row.link), idPart(row.sourceModuleKey), idPart(row.artifactID), idPart(row.programID),
		idPart(row.importID), idPart(row.requestID), idPart(row.valueID), keyPart(uint32(row.requestKey)), idPart(row.targetModuleKey),
	)
	return id
}

func cacheIngressID(row CacheIngress) identity.ContentID {
	if !row.link.Available() || !row.importID.Available() || !row.requestID.Available() || !row.sourceModuleKey.Available() || !row.targetModuleKey.Available() ||
		!row.fromRootID.Available() || !row.toRootID.Available() || !row.actorID.Available() || !row.representativeInstanceID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID(
		"analysis/schema/module-composition/cache-ingress/v1",
		idPart(row.link), idPart(row.importID), idPart(row.requestID), idPart(row.sourceModuleKey), idPart(row.targetModuleKey),
		idPart(row.fromRootID), idPart(row.toRootID), idPart(row.actorID), idPart(row.representativeInstanceID),
	)
	return id
}

func initGenerationID(row InitGeneration) identity.ContentID {
	if !row.link.Available() || !row.ingressID.Available() || !row.moduleKey.Available() || !row.artifactID.Available() || !row.programID.Available() || !row.bodyID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-generation/v1", idPart(row.link), idPart(row.ingressID), idPart(row.moduleKey), idPart(row.artifactID), idPart(row.programID), idPart(row.bodyID))
	return id
}

func initOutcomeID(row InitOutcome) identity.ContentID {
	if !row.link.Available() || !row.generationID.Available() || !row.outcomeID.Available() || !admittedOutcome(row.kind) {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-outcome/v1", idPart(row.link), idPart(row.generationID), idPart(row.outcomeID), kindPart(row.kind), ordinalPart(row.ordinal))
	return id
}

func initTerminalID(row InitTerminal) identity.ContentID {
	if !row.link.Available() || !row.generationID.Available() || !row.outcomeID.Available() {
		return identity.ContentID{}
	}
	id, _ := identity.DeriveContentID("analysis/schema/module-composition/init-terminal/v1", idPart(row.link), idPart(row.generationID), idPart(row.outcomeID))
	return id
}

func idPart(id identity.ContentID) []byte { return id[:] }

func keyPart(key uint32) []byte {
	var part [4]byte
	binary.BigEndian.PutUint32(part[:], key)
	return part[:]
}

func kindPart(kind programschema.OutcomeKind) []byte { return []byte{byte(kind)} }

func ordinalPart(ordinal uint32) []byte {
	var part [4]byte
	binary.BigEndian.PutUint32(part[:], ordinal)
	return part[:]
}
