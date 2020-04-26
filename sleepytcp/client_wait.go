package sleepytcp

import "sync"

type waitingCaller struct {
	ready chan struct{}
	mu    sync.Mutex // protects conn, err, close(ready)
	conn  *clientConn
	err   error
}

// waiting reports whether w is still waiting for an answer (connection or error).
func (w *waitingCaller) waiting() bool {
	select {
	case <-w.ready:
		return false
	default:
		return true
	}
}

// tryDeliver attempts to deliver conn, err to w and reports whether it succeeded.
func (w *waitingCaller) tryDeliver(conn *clientConn, err error) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.conn != nil || w.err != nil {
		return false
	}

	w.conn = conn
	w.err = err

	close(w.ready)

	return true
}

// cancel marks w as no longer wanting a result (for example, due to cancellation).
// If a connection has been delivered already, cancel returns it with c.tryRecycleClientConn.
func (w *waitingCaller) cancel(c *HostClient, err error) {
	w.mu.Lock()
	if w.conn == nil && w.err == nil {
		close(w.ready) // catch misbehavior in future delivery
	}

	conn := w.conn
	w.conn = nil
	w.err = err
	w.mu.Unlock()

	if conn != nil {
		c.tryRecycleClientConn(conn)
	}
}

type waitingCallerQueue struct {
	// This is a queue, not a deque.
	// It is split into two stages - head[headPos:] and tail.
	// popFront is trivial (headPos++) on the first stage, and
	// pushBack is trivial (append) on the second stage.
	// If the first stage is empty, popFront can swap the
	// first and second stages to remedy the situation.
	//
	// This two-stage split is analogous to the use of two lists
	// in Okasaki's purely functional queue but without the
	// overhead of reversing the list when swapping stages.
	head    []*waitingCaller
	headPos int
	tail    []*waitingCaller
}

// len returns the number of items in the queue.
func (q *waitingCallerQueue) len() int {
	return len(q.head) - q.headPos + len(q.tail)
}

// pushBack adds w to the back of the queue.
func (q *waitingCallerQueue) pushBack(w *waitingCaller) {
	q.tail = append(q.tail, w)
}

// popFront removes and returns the waitingCaller at the front of the queue.
func (q *waitingCallerQueue) popFront() *waitingCaller {
	if q.headPos >= len(q.head) {
		if len(q.tail) == 0 {
			return nil
		}

		// Pick up tail as new head, clear tail.
		q.head, q.headPos, q.tail = q.tail, 0, q.head[:0]
	}

	w := q.head[q.headPos]

	q.head[q.headPos] = nil
	q.headPos++

	return w
}

// peekFront returns the waitingCaller at the front of the queue without removing it.
func (q *waitingCallerQueue) peekFront() *waitingCaller {
	if q.headPos < len(q.head) {
		return q.head[q.headPos]
	}
	if len(q.tail) > 0 {
		return q.tail[0]
	}
	return nil
}

// cleanFront pops any wantConns that are no longer waiting from the head of the
// queue, reporting whether any were popped.
func (q *waitingCallerQueue) clearFront() (cleaned bool) {
	for {
		w := q.peekFront()
		if w == nil || w.waiting() {
			return cleaned
		}
		q.popFront()
		cleaned = true
	}
}
