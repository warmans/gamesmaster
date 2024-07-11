package main

import (
	"github.com/warmans/gamesmaster/cmd/cmd"
	"log/slog"
	"os"
)

func main() {
	logger := slog.Default()
	if err := cmd.Execute(logger); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
