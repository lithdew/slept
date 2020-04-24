package main

import (
	"bufio"
	"net"
	"strconv"
)

func main() {
	conn, err := net.Dial("udp", "127.0.0.1:4444")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	bw := bufio.NewWriterSize(conn, 1500)

	for i := 0; i < 100000; i++ {
		if _, err := bw.WriteString(strconv.FormatUint(uint64(i), 10) + "\n"); err != nil {
			panic(err)
		}
	}

	if err := bw.Flush(); err != nil {
		panic(err)
	}
}
