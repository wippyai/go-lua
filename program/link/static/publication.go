package static

import "github.com/wippyai/go-lua/program/semanticsource"

// Publications exposes the sole remaining LinkStatic source fact: the exact
// number of detached mount namespaces. No Program-derived relation survives.
func (v Cold) Publications() ([]semanticsource.Publication, bool) {
	if !v.live() { return nil, false }
	definition, ok := semanticsource.Definition(semanticsource.OriginLinkStatic, 0)
	if !ok { return nil, false }
	publication, err := semanticsource.SealPublication(definition, len(v.schema))
	if err != nil { return nil, false }
	return []semanticsource.Publication{publication}, true
}
