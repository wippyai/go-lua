package source

import packdomain "github.com/wippyai/go-lua/domain/pack"

func sourceContent(source packdomain.Source) (packdomain.Source, [32]byte, bool) {
	id, ok := source.ContentID()
	return source, [32]byte(id), ok && id.Available()
}
