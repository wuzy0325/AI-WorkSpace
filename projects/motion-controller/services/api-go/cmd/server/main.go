package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"motion-controller/services/api-go/api"
	"motion-controller/services/api-go/pkg/appcontext"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	appCtx, err := appcontext.NewAppContext("")
	if err != nil {
		log.Fatalf("Failed to create app context: %v", err)
	}

	if _, err := appCtx.MotionManager.LoadProfiles(); err != nil {
		log.Printf("Warning: failed to load motion profiles: %v", err)
	}

	handler := api.NewRouter(api.Deps{
		MotionManager: appCtx.MotionManager,
	})

	addr := "127.0.0.1:8900"
	if envAddr := os.Getenv("MOTION_SERVER_ADDR"); envAddr != "" {
		addr = envAddr
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	go func() {
		log.Printf("Motion Controller server starting on %s", addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}
