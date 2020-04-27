package main

import (
	"flag"
	"fmt"
	"github.com/lithdew/sleepy"
	"log"
	"net"
	"time"
)

var client bool
var addr string

func check(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func now() float64 {
	return float64(time.Now().UnixNano()) / (1000 * 1000 * 1000)
}

func update(c *sleepy.Channel) {
	ticker := time.NewTicker(16 * time.Millisecond)
	defer ticker.Stop()

	tick := now()

	for range ticker.C {
		check(c.Update(tick))
		tick += 16.0 / 1000
	}
}

func read(c *sleepy.Channel, conn *net.UDPConn) {
	for {
		buf := make([]byte, 65536)

		n, _, err := conn.ReadFrom(buf)
		check(err)

		c.Read(buf[:n])
	}
}

func write(c *sleepy.Channel, conn *net.UDPConn) {
	for b := range c.Out() {
		n, err := conn.WriteTo(b, conn.LocalAddr())
		check(err)

		if n != len(b) {
			check(fmt.Errorf("wrote only %d byte(s), but buf is %d byte(s)", n, len(b)))
		}
	}
}

func main() {
	flag.BoolVar(&client, "client", false, "run a client")
	flag.StringVar(&addr, "addr", ":4444", "address to listen/dial")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", addr)
	check(err)

	c := sleepy.NewChannel(nil)

	if !client {
		conn, err := net.ListenUDP("udp", addr)
		check(err)

		go read(c, conn)
		go write(c, conn)
		go update(c)

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			c.Write([]byte("hello!"))
		}
	}
}
