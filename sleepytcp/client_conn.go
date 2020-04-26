package sleepytcp

import (
	"net"
	"sync"
	"time"
)

type clientConn struct {
	conn        net.Conn
	lastUseTime time.Time
}

var clientConnPool sync.Pool

func acquireClientConn(conn net.Conn) *clientConn {
	v := clientConnPool.Get()
	if v == nil {
		v = &clientConn{}
	}
	cc := v.(*clientConn)
	cc.conn = conn
	return cc
}

func releaseClientConn(cc *clientConn) {
	*cc = clientConn{}
	clientConnPool.Put(cc)
}
