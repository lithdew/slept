package sleepytcp

import (
	"errors"
	"sync"
	"time"
)

const (
	DefaultMaxConnsPerHost           = 512
	DefaultMaxIdleConnDuration       = 10 * time.Second
	DefaultMaxIdempotentCallAttempts = 5

	DefaultReadBufferSize  = 4096
	DefaultWriteBufferSize = 4096
)

var (
	ErrConnectionClosed = errors.New("connection closed")
	ErrNoFreeConns      = errors.New("no free connections are available")
	ErrTimeout          = errors.New("timed out")
)

var startTimeUnix = time.Now().Unix()

type Client struct {
	Dial          DialFunc
	DialDualStack bool

	MaxConnsPerHost           int
	MaxConnWaitTimeout        time.Duration
	MaxIdleConnDuration       time.Duration
	MaxIdempotentCallAttempts int

	ReadBufferSize  int
	WriteBufferSize int

	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	MaxResponseBodySize int

	mu      sync.Mutex
	clients map[string]*HostClient
}

func (c *Client) Do(dst []byte, req *Frame) ([]byte, error) {
	addr, startCleaner := req.addr, false

	c.mu.Lock()

	clients := c.clients

	if clients == nil {
		clients = make(map[string]*HostClient)
		c.clients = clients
	}

	client := clients[addr]

	if client == nil {
		client = &HostClient{
			Addr: addr,

			Dial:          c.Dial,
			DialDualStack: c.DialDualStack,

			MaxConns:                  c.MaxConnsPerHost,
			MaxConnWaitTimeout:        c.MaxConnWaitTimeout,
			MaxIdempotentCallAttempts: c.MaxIdempotentCallAttempts,

			ReadBufferSize:  c.ReadBufferSize,
			WriteBufferSize: c.WriteBufferSize,

			ReadTimeout:  c.ReadTimeout,
			WriteTimeout: c.WriteTimeout,

			MaxResponseBodySize: c.MaxResponseBodySize,
		}

		clients[addr] = client

		if len(clients) == 1 {
			startCleaner = true
		}
	}

	c.mu.Unlock()

	if startCleaner {
		go c.cleanupIdleClients(clients)
	}

	return client.Do(dst, req)
}

func (c *Client) cleanupIdleClients(clients map[string]*HostClient) {
	for {
		c.mu.Lock()

		for host, client := range clients {
			client.mu.Lock()
			idle := client.count == 0
			client.mu.Unlock()

			if idle {
				delete(clients, host)
			}
		}

		stop := len(clients) == 0

		c.mu.Unlock()

		if stop {
			break
		}

		time.Sleep(10 * time.Second)
	}
}
