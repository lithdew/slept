package sleepy

import (
	"errors"
	"io"
	"net"
)

func isEOF(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return netErr.Err.Error() == "use of closed network connection"
	}

	return false
}
