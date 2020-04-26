package main

import (
	"fmt"
	"github.com/lithdew/sleepy/sleepytcp"
	"time"
)

func main() {
	var client sleepytcp.Client

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	f := sleepytcp.AcquireFrame()
	defer sleepytcp.ReleaseFrame(f)

	f.SetAddr("127.0.0.1:4444")
	f.SetBody([]byte("hello\n"))

	for range ticker.C {
		res, err := client.Do(nil, f)
		if err != nil {
			panic(err)
		}

		if len(res) != 0 {
			fmt.Println(string(res[:len(res)-1]))
		}
	}
}
