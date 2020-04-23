package sleepy

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type HostClient struct {
	Addr string

	Dial          DialFunc
	DialDualStack bool

	MaxConns                  int
	MaxConnWaitTimeout        time.Duration
	MaxIdleConnDuration       time.Duration
	MaxIdempotentCallAttempts int

	ReadBufferSize  int
	WriteBufferSize int

	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	MaxResponseBodySize int

	mu sync.Mutex

	// Slice of all available idle connections. Should they idle too long, they get cleaned up in
	// cleanupIdleConnections().
	conns []*clientConn

	// Total number of pending/open connections for this client.
	count int

	// Queue of all callers waiting for an available connection.
	queue *waitingCallerQueue

	// Whether or not cleanupIdleConnections() is running in the background.
	cleanerRunning bool

	// Total number of pending concurrent requests.
	pendingRequests int32

	// The last Unix time this client was used.
	lastUseTime uint32
}

func (c *HostClient) PendingRequests() int {
	return int(atomic.LoadInt32(&c.pendingRequests))
}

func (c *HostClient) LastUseTime() time.Time {
	return time.Unix(startTimeUnix+int64(atomic.LoadUint32(&c.lastUseTime)), 0)
}

func (c *HostClient) Do(req *Request, res *Response) error {
	var (
		err   error
		retry bool
	)

	maxAttempts := c.MaxIdempotentCallAttempts
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxIdempotentCallAttempts
	}

	atomic.AddInt32(&c.pendingRequests, 1)

	// Attempt to retry the request maxAttempts times should the request either be idempotent, or if the underlying
	// connection that the request was being delivered on was closed.

	for attempts := 0; attempts < maxAttempts; maxAttempts++ {
		retry, err = c.do(req, res)
		if err == nil || !retry {
			break
		}

		if !req.Idempotent && err != io.EOF {
			break
		}
	}

	atomic.AddInt32(&c.pendingRequests, -1)

	if errors.Is(err, io.EOF) {
		err = ErrConnectionClosed
	}

	return err
}

func (c *HostClient) do(req *Request, res *Response) (bool, error) {
	if res == nil {
		res = AcquireResponse()
		defer ReleaseResponse(res)
	}

	atomic.StoreUint32(&c.lastUseTime, uint32(time.Now().Unix()-startTimeUnix))

	cc, err := c.tryAcquireClientConn(req.Timeout)
	if err != nil {
		return false, err
	}

	conn := cc.conn

	if c.WriteTimeout > 0 {
		if err = conn.SetWriteDeadline(time.Now().Add(c.WriteTimeout)); err != nil {
			c.destroyClientConn(cc)
			return true, err
		}
	}

	// TODO(kenta): write request data here.

	if c.ReadTimeout > 0 {
		if err = conn.SetReadDeadline(time.Now().Add(c.ReadTimeout)); err != nil {
			c.destroyClientConn(cc)
			return true, err
		}
	}

	// TODO(kenta): read response data here.

	c.tryRecycleClientConn(cc)

	return false, err
}

func (c *HostClient) queueWaitingCaller(caller *waitingCaller) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.queue == nil {
		c.queue = &waitingCallerQueue{}
	}

	c.queue.clearFront()
	c.queue.pushBack(caller)
}

func (c *HostClient) tryAcquireClientConn(timeout time.Duration) (cc *clientConn, err error) {
	var (
		createConn   bool
		startCleaner bool
	)

	// If an idle connection is available, provision it and mark it to be reserved for the caller. Otherwise,
	// if we have not yet the max number of connections this client may create, establish a new one. If we are
	// to establish a new connection and the idle connection cleanup worker is not running, run it.

	c.mu.Lock()

	n := len(c.conns)
	if n == 0 {
		maxConns := c.MaxConns
		if maxConns <= 0 {
			maxConns = DefaultMaxConnsPerHost
		}

		if c.count < maxConns {
			createConn = true

			c.count++

			if !c.cleanerRunning {
				c.cleanerRunning, startCleaner = true, true
			}
		}
	} else {
		n--
		cc = c.conns[n]
		c.conns[n] = nil
		c.conns = c.conns[:n]
	}

	c.mu.Unlock()

	if cc != nil {
		return cc, nil
	}

	// If we are not creating a new connection, and no idle connection is available (all connections are in use), then
	// wait for MaxConnWaitTimeout until a connection is available.

	if !createConn {
		// Wait for min(timeout waiting for available connection, request timeout) seconds for an available
		// connection to perform a read/write against.

		waitDuration := c.MaxConnWaitTimeout

		if waitDuration <= 0 {
			return cc, ErrNoFreeConns
		}

		waitDurationOverridden := false

		if waitDurationOverridden = timeout > 0 && timeout < waitDuration; waitDurationOverridden {
			waitDuration = timeout
		}

		timer := AcquireTimer(waitDuration)
		defer ReleaseTimer(timer)

		// Enter the caller into the waiting queue.

		caller := &waitingCaller{ready: make(chan struct{}, 1)}
		defer func() {
			if err != nil {
				caller.cancel(c, err)
			}
		}()

		c.queueWaitingCaller(caller)

		// If a connection becomes available, return it to the caller. If the caller is waiting too long,
		// then either timeout if the caller was making a request, or tell the caller they waited too long
		// and that there are no free connections available for them.

		select {
		case <-caller.ready:
			return caller.conn, caller.err
		case <-timer.C:
			if waitDurationOverridden {
				return cc, ErrTimeout
			}
			return cc, ErrNoFreeConns
		}
	}

	// Start cleaning up idle connections.

	if startCleaner {
		go c.cleanupIdleConnections()
	}

	// Initialize the connection.

	conn, err := dialAddr(c.Addr, c.Dial, c.DialDualStack)
	if err != nil {
		// Either decrease total open/pending connections, or if a waiting caller is available, start dialing one
		// for them.
		c.decrementCountOrTryDialForWaitingCaller()

		return cc, err
	}

	cc = acquireClientConn(conn)

	return cc, err
}

func (c *HostClient) tryRecycleClientConn(cc *clientConn) {
	cc.lastUseTime = time.Now()

	// If no wait timeout is specified, immediately push the *clientConn to the list of available
	// idle connections.

	if c.MaxConnWaitTimeout <= 0 {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.conns = append(c.conns, cc)

		return
	}

	// If there are callers waiting and timing out for an open connection, try deliver this *clientConn to them. Else,
	// push it back to the list of available idle connections.

	c.mu.Lock()
	defer c.mu.Unlock()

	delivered := false

	if queue := c.queue; queue != nil && queue.len() > 0 {
		for queue.len() > 0 {
			if caller := queue.popFront(); caller.waiting() {
				delivered = caller.tryDeliver(cc, nil)
				break
			}
		}
	}

	if !delivered {
		c.conns = append(c.conns, cc)
	}
}

func (c *HostClient) tryDialForWaitingCaller(caller *waitingCaller) {
	conn, err := dialAddr(c.Addr, c.Dial, c.DialDualStack)
	if err != nil {
		// Notify to the caller that there was an error dialing the connection.
		caller.tryDeliver(nil, err)

		// Either decrease total open/pending connections, or if a waiting caller is available, start dialing one
		// for them.
		c.decrementCountOrTryDialForWaitingCaller()

		return
	}

	cc := acquireClientConn(conn)

	// Try deliver the acquired *clientConn to the caller. If somehow an error or *clientConn was already
	// provided to the caller however, release this acquired *clientConn.

	if delivered := caller.tryDeliver(cc, nil); !delivered {
		c.tryRecycleClientConn(cc)
	}
}

func (c *HostClient) decrementCountOrTryDialForWaitingCaller() {
	// If no wait timeout is specified, which implies that there cannot be any waiting callers, immediately decrease
	// the number of open/pending connections.

	if c.MaxConnWaitTimeout <= 0 {
		c.mu.Lock()
		defer c.mu.Unlock()

		c.count--

		return
	}

	// If there are callers waiting and timing out for a pending/open connection, try dial a connection for them. If
	// there are no callers, decrease the number of open/pending connections.

	c.mu.Lock()
	defer c.mu.Unlock()

	dialing := false

	if queue := c.queue; queue != nil && queue.len() > 0 {
		for queue.len() > 0 {
			if caller := queue.popFront(); caller.waiting() {
				go c.tryDialForWaitingCaller(caller)
				dialing = true
				break
			}
		}
	}

	if !dialing {
		c.count--
	}
}

func (c *HostClient) destroyClientConn(cc *clientConn) {
	// Either decrease total open/pending connections, or if a waiting caller is available, start dialing one for them.
	c.decrementCountOrTryDialForWaitingCaller()

	// Close *clientConn's underlying connection.
	cc.conn.Close()

	// Release resources.
	releaseClientConn(cc)
}

func (c *HostClient) cleanupIdleConnections() {
	var scratch []*clientConn

	maxIdleConnDuration := c.MaxIdleConnDuration
	if maxIdleConnDuration <= 0 {
		maxIdleConnDuration = DefaultMaxIdleConnDuration
	}

	for {
		currentTime := time.Now()

		// Determine idle connections to be closed.

		c.mu.Lock()

		conns := c.conns
		i, n := 0, len(conns)

		// Find the first connection that has been idle more than the max idle duration.

		for i < n && currentTime.Sub(conns[i].lastUseTime) > maxIdleConnDuration {
			i++
		}

		// If an idle connection has been found, sleep for (max idle duration - idle duration + 1) after cleaning
		// found idle connections. Else, sleep for max idle duration.

		duration := maxIdleConnDuration
		if i < n {
			duration = maxIdleConnDuration - currentTime.Sub(conns[i].lastUseTime) + 1
		}

		// Put all connections that have been marked to have been idle for too long so far into scratch.

		scratch = append(scratch[:0], conns[:i]...)

		if i > 0 {
			m := copy(conns, conns[i:])
			for i = m; i < n; i++ {
				conns[i] = nil
			}

			c.conns = conns[:m]
		}

		c.mu.Unlock()

		// Close idle connections.

		for i := range scratch {
			c.destroyClientConn(scratch[i])
			scratch[i] = nil
		}

		// Stop this cleaning goroutine if no connections are left.

		c.mu.Lock()

		mustStop := c.count == 0
		if mustStop {
			c.cleanerRunning = false
		}

		c.mu.Unlock()

		if mustStop {
			break
		}

		// If this cleanup goroutine is to not stop, wait the duration we expect until a connection
		// might possibly be idle and thus meant to be cleaned up.

		time.Sleep(duration)
	}
}
