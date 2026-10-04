// Command server runs the Chase game: static frontend, REST bootstrap API and
// the WebSocket game endpoint, all from one process.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chase/internal/bank"
	"chase/internal/httpapi"
	"chase/internal/room"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := env("PORT", "8080")
	staticDir := env("STATIC_DIR", "./web")
	questionsDir := os.Getenv("QUESTIONS_DIR") // empty = built-in banks

	set, err := bank.Load(questionsDir)
	if err != nil {
		log.Fatalf("question bank: %v", err)
	}
	log.Printf("question bank: %d cash builder, %d head-to-head, %d + %d final",
		len(set.CashBuilder), len(set.HeadToHead), len(set.FinalA), len(set.FinalB))

	cfg := room.DefaultConfig()
	if os.Getenv("FAST_MODE") == "true" {
		// Short clocks so a whole game can be played through in a minute while testing.
		cfg.CBSeconds, cfg.FinalSeconds = 6, 8
		cfg.LockWindow, cfg.RevealFor = 2*time.Second, 1200*time.Millisecond
		log.Println("FAST_MODE: short clocks enabled (testing only)")
	}
	mgr := room.NewManager(set, cfg, 200)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           httpapi.New(mgr, staticDir),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	mgr.Shutdown()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
