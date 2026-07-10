package state

func requireFiniteLaneForWrite(top bool, op, subject, lane string) {
	if top {
		panic("state: cannot " + op + " " + subject + " into top " + lane + " lane")
	}
}

func requireNonBottomLaneValue(bottom bool, lane, subject string) {
	if bottom {
		panic("state: " + lane + " lane with requires non-bottom " + subject)
	}
}
