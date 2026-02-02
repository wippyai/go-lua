package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// parsePath parses a path string into a constraint.Path for display purposes.
//
// This function parses path syntax like "x", "x.field", "x[0]", or "x[\"key\"]"
// into a structured Path representation. The parsing supports:
//
//   - Simple identifiers: "x" becomes Path{Root: "x"}
//   - Field access: "x.foo" becomes Path{Root: "x", Segments: [.foo]}
//   - Integer index: "x[0]" becomes Path{Root: "x", Segments: [[0]]}
//   - String index: "x[\"key\"]" becomes Path{Root: "x", Segments: [["key"]]}
//
// The returned Path has Symbol=0 and MUST NOT be used for value identity in
// the flow solver. For identity-aware path resolution that binds to SSA versions,
// use the pathkey.Resolver or binding-aware extractors in flowbuild/path.
//
// Returns (Path, true) on successful parse, or (empty Path, false) on error.
// The returned Path has Symbol=0 and MUST NOT be used for value identity in the solver.
// For identity-aware paths, use PathFromExprFull with a SymbolResolver.
func parsePath(path string) (constraint.Path, bool) {
	if path == "" {
		return constraint.Path{}, false
	}

	i := 0

	root := pathkey.ReadIdent(path, &i)
	if root == "" {
		return constraint.Path{}, false
	}

	var segs []constraint.Segment

	for i < len(path) {
		switch path[i] {
		case '.':
			i++

			name := pathkey.ReadIdent(path, &i)
			if name == "" {
				return constraint.Path{}, false
			}

			segs = append(segs, constraint.Segment{Kind: constraint.SegmentField, Name: name})
		case '[':
			i++
			if i < len(path) && path[i] == '"' {
				i++

				var out []byte

				for i < len(path) {
					ch := path[i]
					if ch == '\\' && i+1 < len(path) {
						out = append(out, path[i+1])
						i += 2

						continue
					}

					if ch == '"' {
						break
					}

					out = append(out, ch)
					i++
				}

				if i >= len(path) || path[i] != '"' {
					return constraint.Path{}, false
				}

				i++
				if i >= len(path) || path[i] != ']' {
					return constraint.Path{}, false
				}

				i++

				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: string(out)})

				continue
			}

			start := i

			for i < len(path) && path[i] != ']' {
				i++
			}

			if i >= len(path) || path[i] != ']' {
				return constraint.Path{}, false
			}

			token := path[start:i]
			i++

			if idx, ok := pathkey.ParseIntLiteral(token); ok {
				segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx})
				continue
			}

			if token == "" {
				return constraint.Path{}, false
			}

			segs = append(segs, constraint.Segment{Kind: constraint.SegmentIndexString, Name: token})
		default:
			return constraint.Path{}, false
		}
	}

	return constraint.Path{Root: root, Segments: segs}, true
}
