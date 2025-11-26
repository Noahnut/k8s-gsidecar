package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)

	go func() {
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Received signal to exit, cancelling context")
		cancel()
	}()

	sideCar := New(ctx)
	log.Println("Running SideCar")
	sideCar.Run()

	log.Println("SideCar exited")
}
