package state

func requireFiniteLaneForWrite(top bool, op, subject, lane string) {
	if top {
		panic("state: cannot " + op + " " + subject + " into top " + lane + " lane")
	}
}
