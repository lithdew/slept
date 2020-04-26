package main

import (
	"crypto/rand"
	"flag"
	"github.com/lithdew/sleepy"
	"log"
	"math"
	"net"
	"time"
)

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func endpointRecv(e *sleepy.Endpoint, conn net.PacketConn) {
	buf := make([]byte, math.MaxUint16)

	for {
		n, _, err := conn.ReadFrom(buf[:])
		check(err)

		check(e.RecvPacket(buf[:n]))

		log.Printf("recv %d byte(s)", n)
	}
}

var client bool

func runClient(addr *net.UDPAddr) {
	conn, err := net.DialUDP("udp", nil, addr)
	check(err)
	defer func() {
		check(conn.Close())
	}()

	e := sleepy.NewEndpoint(conn)
	go endpointRecv(e, conn)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	buf := make([]byte, 4096)

	for range ticker.C {
		_, err = rand.Read(buf)
		check(err)

		_, err = e.SendPacket(buf)

		log.Printf("sent %d byte(s)", len(buf))
	}
}

func runServer(addr *net.UDPAddr) {
	conn, err := net.ListenUDP("udp", addr)
	check(err)
	defer func() {
		check(conn.Close())
	}()

	e := sleepy.NewEndpoint(conn)
	endpointRecv(e, conn)
}

func main() {
	flag.BoolVar(&client, "client", false, "client mode")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", ":4444")
	check(err)

	if client {
		log.Println("running as client")
		runClient(addr)
	} else {
		log.Println("running as server")
		runServer(addr)
	}
}
