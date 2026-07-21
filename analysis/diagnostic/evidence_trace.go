package diagnostic

// EvidenceLine is the text-only evidence projection shared by renderers that
// cannot display a source frame. Heading and Message intentionally use the
// same vocabulary and witness-trace ordering as Render.
type EvidenceLine struct {
	Heading string
	Message string
}

// EvidenceTrace renders a causal evidence chain without terminal framing.
// It is suitable for protocol renderers such as hover: source-frame terminal
// rendering remains Render's responsibility, while the proof ordering and
// proven/claimed/missing-proof vocabulary stay identical.
func EvidenceTrace(items []Evidence) []EvidenceLine {
	ordered := SourceOrderedEvidenceTrace(items, "")
	if len(ordered) == 0 {
		return nil
	}
	style := newRenderStyle(false)
	out := make([]EvidenceLine, 0, len(ordered))
	for _, item := range ordered {
		out = append(out, EvidenceLine{
			Heading: style.evidenceHeadingText(item),
			Message: evidenceMessage(item),
		})
	}
	return out
}
