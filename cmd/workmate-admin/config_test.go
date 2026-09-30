package main

import "testing"

func TestServerConfigEnvironmentAndFlagPriority(t *testing.T) {
	env := map[string]string{"WORKMATE_LISTEN": "0.0.0.0:8090", "WORKMATE_DATA_DIR": "cloud-data", "WORKMATE_PUBLIC_URL": "https://pro.example.com/", "WORKMATE_TRUSTED_PROXIES": "127.0.0.1,::1", "WORKMATE_DOWNLOAD_URL": "https://example.com/workmate.zip"}
	get := func(key string) string { return env[key] }
	cfg, err := readConfig(nil, get)
	if err != nil || cfg.publicURL != "https://pro.example.com" || cfg.dataDir != "cloud-data" || cfg.downloadURL != "https://example.com/workmate.zip" || len(cfg.trustedProxies) != 2 {
		t.Fatal("production environment was not applied", cfg, err)
	}
	cfg, err = readConfig([]string{"-listen", "127.0.0.1:9999", "-data-dir", "local-data", "-public-url", "https://new.example.com", "-trusted-proxies", "", "-download-url", "https://release.example.com/new.zip", "-init"}, get)
	if err != nil || cfg.listen != "127.0.0.1:9999" || cfg.dataDir != "local-data" || cfg.publicURL != "https://new.example.com" || cfg.downloadURL != "https://release.example.com/new.zip" || len(cfg.trustedProxies) != 0 || !cfg.initOnly {
		t.Fatal("explicit flags must override environment", cfg, err)
	}
	cfg, err = readConfig(nil, func(string) string { return "" })
	if err != nil || cfg.listen != "127.0.0.1:8090" || cfg.dataDir != "admin-data" || cfg.publicURL != "" || len(cfg.trustedProxies) != 0 {
		t.Fatal("local defaults changed", cfg, err)
	}
}
func TestServerConfigRejectsInvalidDeployment(t *testing.T) {
	for _, args := range [][]string{
		{"-listen", "0.0.0.0:8090"}, {"-listen", ":8090"}, {"-listen", "localhost:70000"}, {"-listen", "localhost:not-a-port"},
		{"-public-url", "http://pro.example.com"}, {"-public-url", "https://user:secret@pro.example.com"}, {"-public-url", "https://pro.example.com/path"},
		{"-public-url", "https://pro.example.com?"}, {"-public-url", "https://pro.example.com#fragment"}, {"-download-url", "http://example.com/app.zip"}, {"-download-url", "https://user:pass@example.com/app.zip"}, {"-data-dir", ""},
		{"-trusted-proxies", "*"}, {"-trusted-proxies", "0.0.0.0/0"}, {"-trusted-proxies", "127.0.0.1,"}, {"unexpected"},
	} {
		if _, err := readConfig(args, func(string) string { return "" }); err == nil {
			t.Errorf("invalid configuration accepted: %q", args)
		}
	}
}
