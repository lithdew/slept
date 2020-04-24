package main

import (
	"fmt"
	"github.com/lithdew/sleepy"
	"time"
)

func main() {
	var client sleepy.Client

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	f := sleepy.AcquireFrame()
	defer sleepy.ReleaseFrame(f)

	f.SetAddr("127.0.0.1:4444")
	f.SetBody([]byte("hello\n"))

	for range ticker.C {
		res, err := client.Do(nil, f)
		if err != nil {
			panic(err)
		}

		fmt.Println(string(res[:len(res)-1]))
	}
}
