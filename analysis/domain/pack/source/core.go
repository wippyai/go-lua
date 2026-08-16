package source

import packdomain "github.com/wippyai/go-lua/analysis/domain/pack"

func sourceContent(source packdomain.Source) (packdomain.Source, [32]byte, bool) {
	id, ok := source.ContentID()
	return source, [32]byte(id), ok && id.Available()
}

func sealedResult(schema *packdomain.Schema, source packdomain.Source) (packdomain.Root, packdomain.Value, bool) {
	if schema == nil {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	receipt, ok := schema.SourceResult(source)
	if !ok || !schema.OwnsSourceResult(receipt) {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	issued, issuedOK := receipt.Source()
	root, rootOK := receipt.Root()
	value, valueOK := receipt.Value()
	if !issuedOK || issued != source || !rootOK || !valueOK || !schema.Admit(root, value) {
		return packdomain.Root{}, packdomain.Value{}, false
	}
	return root, value, true
}
