package sleepy

import (
	"sync"
	"time"
)

type Request struct {
	// Address to send this request to.
	Addr string

	// Whether or not this request may be retried if it fails to be delivered.
	Idempotent bool

	// Total duration to wait for until we consider this request to have timed out.
	Timeout time.Duration
}

func (r *Request) Reset() {
	// TODO(kenta): implement
}

type Response struct{}

func (r *Response) Reset() {
	// TODO(kenta): implement
}

var (
	requestPool  sync.Pool
	responsePool sync.Pool
)

func AcquireRequest() *Request {
	v := requestPool.Get()
	if v == nil {
		return &Request{}
	}
	return v.(*Request)
}

func ReleaseRequest(req *Request) {
	req.Reset()
	requestPool.Put(req)
}

func AcquireResponse() *Response {
	v := responsePool.Get()
	if v == nil {
		return &Response{}
	}
	return v.(*Response)
}

func ReleaseResponse(res *Response) {
	res.Reset()
	responsePool.Put(res)
}
