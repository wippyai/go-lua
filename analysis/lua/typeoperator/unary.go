package typeoperator

func unaryMetamethodName(op string) (string, bool) {
	switch op {
	case "-":
		return "__unm", true
	case "~":
		return "__bnot", true
	case "#":
		return "__len", true
	default:
		return "", false
	}
}
