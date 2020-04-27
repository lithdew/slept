package main

import (
	"flag"
)

var client bool
var addr string

func main() {
	flag.BoolVar(&client, "client", false, "run a client")
	flag.StringVar(&addr, "addr", ":4444", "address to listen/dial")
	flag.Parse()

	//c := sleepy.NewChannel(nil)
	//
	//if client {
	//	conn, err := c.
	//}
}
