package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"workmate/internal/admin"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "HTTP listen address")
	data := flag.String("data-dir", "admin-data", "database, credentials and license key directory")
	public := flag.String("public-url", "", "external HTTPS origin when deployed behind a reverse proxy")
	initOnly := flag.Bool("init", false, "initialize local database and credentials, then exit")
	flag.Parse()
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		log.Fatal("Invalid listen address")
	}
	ip := net.ParseIP(host)
	loopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if *public != "" {
		u, e := url.Parse(*public)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			log.Fatal("public-url must be an HTTPS origin, e.g. https://pro.example.com")
		}
	}
	if !loopback && *public == "" {
		log.Fatal("Non-loopback access requires -public-url https://your-domain (deploy behind an HTTPS reverse proxy)")
	}
	abs, err := filepath.Abs(*data)
	if err != nil {
		log.Fatal(err)
	}
	s, err := admin.Open(abs)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()
	fmt.Println("WorkMate admin data:", abs)
	if _, err = os.Stat(filepath.Join(abs, "initial-password.txt")); err == nil {
		fmt.Println("Account: admin. Read the initial password from:", filepath.Join(abs, "initial-password.txt"))
		fmt.Println("After logging in, change the password in Settings.")
	}
	fmt.Println("Client build public key:", filepath.Join(abs, "license-public.key"))
	if *initOnly {
		return
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	srv := &http.Server{Handler: admin.NewHandler(s, admin.HTTPOptions{PublicURL: *public}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	fmt.Println("Admin:", "http://"+listener.Addr().String()+"/admin")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	shutdownDone := make(chan struct{})
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		close(shutdownDone)
	}()
	if err = srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-shutdownDone
}
