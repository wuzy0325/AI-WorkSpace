package main

import (
	"log/slog"
	"os"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	app := NewWalkApp()
	defer app.Cleanup()

	mw, err := CreateMainWindow(app)
	if err != nil {
		slog.Error("create main window failed", "err", err)
		os.Exit(1)
	}

	app.SetMainWindow(mw)

	slog.Info("win7 daq-mvp starting")
	mw.Run()
	slog.Info("win7 daq-mvp exited")
}
