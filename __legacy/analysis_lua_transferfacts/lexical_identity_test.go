package transferfacts

import "github.com/wippyai/go-lua/analysis/lexicalidentity"

func testLexicalBodyID(name string) lexicalidentity.StableLexicalBodyID {
	return lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("transferfacts-test:" + name)))
}
