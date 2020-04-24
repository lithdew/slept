package sleepy

import (
	"bufio"
	"github.com/valyala/bytebufferpool"
	"sync"
	"time"
)

var (
	framePool     sync.Pool
	frameBodyPool bytebufferpool.Pool
)

func AcquireFrame() *Frame {
	v := framePool.Get()
	if v == nil {
		return &Frame{}
	}
	return v.(*Frame)
}

func ReleaseFrame(req *Frame) {
	req.Reset()
	framePool.Put(req)
}

type Frame struct {
	// Address to send this frame to.
	addr string

	// Whether or not this frame may be retried if it fails to be delivered.
	Idempotent bool

	// Total duration to wait for until we consider this frame to have timed out.
	Timeout time.Duration

	// Body of this frame.
	body *bytebufferpool.ByteBuffer
}

func (r *Frame) bodyBuffer() *bytebufferpool.ByteBuffer {
	if r.body == nil {
		r.body = frameBodyPool.Get()
	}
	return r.body
}

func (r *Frame) bodyBytes() []byte {
	if r.body == nil {
		return nil
	}
	return r.body.B
}

func (r *Frame) WriteTo(dst *bufio.Writer) error {
	body := r.bodyBytes()
	if len(body) == 0 {
		return nil
	}
	_, err := dst.Write(body)
	return err
}

func (r *Frame) AppendBody(b []byte) {
	buf := r.bodyBuffer()
	buf.B = append(buf.B, b...)
}

func (r *Frame) Body() []byte {
	return r.bodyBuffer().B
}

func (r *Frame) SetBody(b []byte) {
	buf := r.bodyBuffer()
	buf.Reset()
	buf.Write(b)
}

func (r *Frame) SetAddr(addr string) {
	r.addr = addr
}

func (r *Frame) Reset() {
	if r.body != nil {
		frameBodyPool.Put(r.body)
		r.body = nil
	}

	r.Timeout = 0
}
