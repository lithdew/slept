package main

import (
	"github.com/lithdew/sleepy"
	"time"
)

func main() {
	var client sleepy.Client

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	req := sleepy.AcquireRequest()
	defer sleepy.ReleaseRequest(req)

	req.Addr = "127.0.0.1:4444"

	for range ticker.C {
		if err := client.Do(req, nil); err != nil {
			panic(err)
		}
	}
}
