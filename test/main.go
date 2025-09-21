package main

import (
	"log"
	"time"
)

func main() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	i := 0
	for range ticker.C {
		log.Println("tick", i)
		i++
	}
}
