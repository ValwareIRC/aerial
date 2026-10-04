// Command aerald runs the Aerial API server.
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aerial-project/aerial/internal/app"
	"github.com/aerial-project/aerial/internal/httpapi"
)

// shutdownGrace is how long in-flight requests have to finish once a signal
// arrives. Nothing here is persisted, but a request half-answered is still a
// worse answer than one refused.
const shutdownGrace = 10 * time.Second

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	seed := flag.Bool("seed", true, "load the demonstration dataset")
	secure := flag.Bool("secure-cookie", false, "mark the session cookie Secure (requires TLS)")
	version := flag.Bool("version", false, "print the version and licence, then exit")
	flag.Parse()

	if *version {
		fmt.Printf("%s %s\n%s\n%s\n%s\n",
			app.Name, app.Version, app.Copyright, app.LicenceName, app.LicenceNotice)
		return
	}

	salt, err := loadSalt()
	if err != nil {
		log.Fatalf("salt: %v", err)
	}

	a, err := app.New(salt)
	if err != nil {
		log.Fatalf("start: %v", err)
	}
	if *seed {
		if err := a.Seed(); err != nil {
			log.Fatalf("seed: %v", err)
		}
		password := os.Getenv("AERIAL_DEV_PASSWORD")
		if password == "" {
			password = "aerial-development"
			log.Printf("AERIAL_DEV_PASSWORD not set; seeded accounts use %q", password)
		}
		if err := a.SeedUsers(password); err != nil {
			log.Fatalf("seed users: %v", err)
		}
		log.Print("accounts: a.reyes (service), j.okafor (enforcement), m.whitlock (admin)")
	}

	srv := &http.Server{
		Addr:    *addr,
		Handler: httpapi.New(a, *secure).Routes(),
		// A client that opens a connection and then says nothing, or reads
		// one byte a minute, must not be able to hold a slot indefinitely.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("%s %s — %s", app.Name, app.Version, app.LicenceNotice)
	log.Printf("aerald listening on %s", *addr)

	stopping := make(chan struct{})
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		sig := <-signals
		log.Printf("%v received; finishing in-flight requests", sig)
		close(stopping)

		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-stopping
	log.Print("stopped")
}

// loadSalt reads AERIAL_SALT, or generates an ephemeral one for development.
// An ephemeral salt makes enforcement tokens unstable across restarts, which is
// wrong for a deployment but safe for a demo.
func loadSalt() ([]byte, error) {
	if s := os.Getenv("AERIAL_SALT"); s != "" {
		return []byte(s), nil
	}
	log.Print("AERIAL_SALT not set; generating an ephemeral salt (tokens will not survive a restart)")
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}
