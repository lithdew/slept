package sleepytcp

import "net"

type DialFunc func(addr string) (net.Conn, error)

func dialAddr(addr string, dial DialFunc, dialDualStack bool) (net.Conn, error) {
	if dial == nil {
		if dialDualStack {
			dial = DialDualStack
		} else {
			dial = Dial
		}
	}

	conn, err := dial(addr)
	if err != nil {
		return nil, err
	}

	if conn == nil {
		panic("BUG: DialFunc did not return any net.Conn nor an error")
	}

	return conn, nil
}
