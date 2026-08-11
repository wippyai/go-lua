package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
)

func cloneSymbols(ids []symbol.ID) []symbol.ID {
	if len(ids) == 0 {
		return nil
	}
	return append([]symbol.ID(nil), ids...)
}

func cloneParamSlots(slots []ParamSlot) []ParamSlot {
	if len(slots) == 0 {
		return nil
	}
	return append([]ParamSlot(nil), slots...)
}

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}

func cloneStaticTypePublications(publications []StaticTypePublication) []StaticTypePublication {
	if len(publications) == 0 {
		return nil
	}
	out := make([]StaticTypePublication, len(publications))
	for i, publication := range publications {
		out[i] = StaticTypePublication{
			Index:  publication.Index,
			Source: append([]string(nil), publication.Source...),
			Alias:  publication.Alias.copy(),
		}
	}
	return out
}
