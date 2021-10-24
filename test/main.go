package main

import (
	"os"
)

func main() {
	/*sigs := make(chan os.Signal)
	signal.Notify(sigs, os.Interrupt)
	go func() {
		for s := range sigs {
			if s == os.Interrupt {
				fmt.Printf("Proxy signal received %+v", s)
				os.Exit(0)
			}
		}
	}()
	time.Sleep(2 * time.Minute)*/

	os.Exit(0)
}
