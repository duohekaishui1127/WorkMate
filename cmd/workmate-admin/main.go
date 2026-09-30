package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"workmate/internal/admin"
)

func main() {
	cfg, err := readConfig(os.Args[1:], os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg); err != nil {
		log.Fatal(err)
	}
}
func run(ctx context.Context, cfg serverConfig) error {
	abs, err := filepath.Abs(cfg.dataDir)
	if err != nil {
		return err
	}
	s, err := admin.Open(abs)
	if err != nil {
		return err
	}
	defer s.Close()
	fmt.Println("WorkMate admin data:", abs)
	if _, err := os.Stat(filepath.Join(abs, "initial-password.txt")); err == nil {
		fmt.Println("Account: admin. Read the initial password from:", filepath.Join(abs, "initial-password.txt"))
		fmt.Println("After logging in, change the password in Settings.")
	}
	fmt.Println("Client build public key:", filepath.Join(abs, "license-public.key"))
	if cfg.initOnly {
		return nil
	}
	listener, err := net.Listen("tcp", cfg.listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	srv := &http.Server{Handler: admin.NewHandler(s, admin.HTTPOptions{PublicURL: cfg.publicURL, TrustedProxies: cfg.trustedProxies}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	adminURL := cfg.publicURL
	if adminURL == "" {
		adminURL = "http://" + listener.Addr().String()
	}
	fmt.Println("Admin:", adminURL+"/admin")
	fmt.Println("HTTP listener:", listener.Addr())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			<-done
			return err
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
