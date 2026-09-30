package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"workmate/internal/admin"
)

type serverConfig struct {
	listen, dataDir, publicURL string
	trustedProxies             []netip.Prefix
	initOnly                   bool
}

func readConfig(args []string, getenv func(string) string) (serverConfig, error) {
	var cfg serverConfig
	value := func(name, fallback string) string {
		if v := strings.TrimSpace(getenv(name)); v != "" {
			return v
		}
		return fallback
	}
	fs := flag.NewFlagSet("workmate-admin", flag.ContinueOnError)
	fs.StringVar(&cfg.listen, "listen", value("WORKMATE_LISTEN", "127.0.0.1:8090"), "HTTP listen address; WORKMATE_LISTEN")
	fs.StringVar(&cfg.dataDir, "data-dir", value("WORKMATE_DATA_DIR", "admin-data"), "database and license key directory; WORKMATE_DATA_DIR")
	fs.StringVar(&cfg.publicURL, "public-url", value("WORKMATE_PUBLIC_URL", ""), "external HTTPS origin; WORKMATE_PUBLIC_URL")
	var proxies string
	fs.StringVar(&proxies, "trusted-proxies", value("WORKMATE_TRUSTED_PROXIES", ""), "comma-separated trusted proxy IPs/CIDRs; WORKMATE_TRUSTED_PROXIES")
	fs.BoolVar(&cfg.initOnly, "init", false, "initialize local data and credentials, then exit")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() != 0 {
		return cfg, errors.New("unexpected arguments; use -help for available flags")
	}
	cfg.listen = strings.TrimSpace(cfg.listen)
	cfg.dataDir = strings.TrimSpace(cfg.dataDir)
	cfg.publicURL = strings.TrimSpace(cfg.publicURL)
	if cfg.dataDir == "" {
		return cfg, errors.New("data-dir cannot be empty")
	}
	host, port, err := net.SplitHostPort(cfg.listen)
	if err != nil {
		return cfg, errors.New("listen must contain a host and numeric port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return cfg, errors.New("listen port must be between 0 and 65535")
	}
	if cfg.publicURL != "" {
		u, err := url.Parse(cfg.publicURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return cfg, errors.New("public-url must be an HTTPS origin, e.g. https://pro.your-domain.com")
		}
		cfg.publicURL = strings.TrimRight(u.String(), "/")
	}
	ip := net.ParseIP(host)
	loopback := strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
	if !loopback && cfg.publicURL == "" {
		return cfg, errors.New("non-loopback access requires public-url and an HTTPS reverse proxy")
	}
	cfg.trustedProxies, err = admin.ParseTrustedProxies(proxies)
	if err != nil {
		return cfg, fmt.Errorf("proxy configuration: %w", err)
	}
	return cfg, nil
}
