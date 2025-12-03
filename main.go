package main

import (
	"context"
	"k8s-gsidecar/logger"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

var l *slog.Logger = logger.GetLogger()

func main() {

	l.Info("Starting SideCar")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)

	go func() {
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		l.Info("Received signal to exit, cancelling context")
		cancel()
	}()

	sideCar := New(ctx)
	l.Info("Running SideCar")
	sideCar.Run()

	l.Info("SideCar exited")
}
