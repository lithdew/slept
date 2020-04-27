package main

import (
	"errors"
	"flag"
	"github.com/lithdew/bytesutil"
	"github.com/lithdew/sleepy"
	"github.com/pkg/profile"
	"io"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"
)

func check(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func now() float64 {
	return float64(time.Now().UnixNano()) / (1000 * 1000 * 1000)
}

func isSafeError(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}

	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return netErr.Err.Error() == "use of closed network connection"
	}

	return false
}

func endpointRecv(wg *sync.WaitGroup, e *sleepy.Endpoint, conn net.PacketConn) {
	defer wg.Done()

	buf := make([]byte, math.MaxUint16)

	for {
		n, _, err := conn.ReadFrom(buf[:])
		if err != nil {
			if isSafeError(err) {
				return
			}
			check(err)
		}

		check(e.RecvPacket(buf[:n]))

		e.Update(now())

		//sent, recv, acked := e.Bandwidth()
		//fmt.Printf("rtt = %vms | packet loss = %v%% | sent = %vkbps | recv = %vkbps | acked = %vkbps\n",
		//	e.RTT(),
		//	int(math.Floor(e.PacketLoss()+.5)),
		//	int(sent), int(recv), int(acked),
		//)

		if enableLogs {
			log.Printf("recv %d byte(s)", n)
		}
	}
}

func endpointSend(wg *sync.WaitGroup, e *sleepy.Endpoint) {
	defer wg.Done()

	buf := make([]byte, 16384)

	for {
		buf = bytesutil.RandomSlice(buf)

		n, err := e.SendPacket(buf)
		if err != nil {
			if isSafeError(err) {
				return
			}
			check(err)
		}

		e.Update(now())

		//sent, recv, acked := e.Bandwidth()
		//fmt.Printf("rtt = %vms | packet loss = %v%% | sent = %vkbps | recv = %vkbps | acked = %vkbps\n",
		//	e.RTT(),
		//	int(math.Floor(e.PacketLoss()+.5)),
		//	int(sent), int(recv), int(acked),
		//)

		if enableLogs {
			log.Printf("sent %d byte(s)", n)
		}
	}
}

var (
	client          bool
	enableLogs      bool
	enableProfiling bool
)

func runClient(addr *net.UDPAddr) {
	conn, err := net.DialUDP("udp", nil, addr)
	check(err)

	var wg sync.WaitGroup
	wg.Add(2)

	e := sleepy.NewEndpoint(conn)
	go endpointRecv(&wg, e, conn)
	go endpointSend(&wg, e)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch

	check(conn.Close())
	wg.Wait()
}

func runServer(addr *net.UDPAddr) {
	conn, err := net.ListenUDP("udp", addr)
	check(err)

	var wg sync.WaitGroup
	wg.Add(1)

	e := sleepy.NewEndpoint(conn)
	go endpointRecv(&wg, e, conn)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch

	check(conn.Close())
	wg.Wait()
}

func main() {
	flag.BoolVar(&client, "client", false, "client mode")
	flag.BoolVar(&enableLogs, "log", false, "log number of bytes sent/recv")
	flag.BoolVar(&enableProfiling, "profile", false, "perform cpu profiling")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", ":4444")
	check(err)

	opts := []func(*profile.Profile){
		profile.CPUProfile,
		profile.NoShutdownHook,
	}

	if client {
		opts = append(opts, profile.ProfilePath("./cmd/udp_test/client"))

		if enableProfiling {
			defer profile.Start(opts...).Stop()
		}

		log.Println("running as client")
		runClient(addr)
	} else {
		opts = append(opts, profile.ProfilePath("./cmd/udp_test/server"))

		if enableProfiling {
			defer profile.Start(opts...).Stop()
		}

		log.Println("running as server")
		runServer(addr)
	}
}
