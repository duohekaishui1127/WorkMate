package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

type CommerceConfig struct {
	Enabled   bool   `json:"enabled"`
	Price     string `json:"price"`
	Contact   string `json:"contact"`
	ServerURL string `json:"server_url,omitempty"`
}

// A closed purchase entry still permits entering an existing activation code.
func loadCommerceConfig(dir string) (CommerceConfig, error) {
	var cfg CommerceConfig
	err := readJSON(filepath.Join(dir, "commerce.json"), &cfg)
	if err != nil && !os.IsNotExist(err) {
		return cfg, errors.New("购买信息暂不可用，请稍后再试。")
	}
	cfg.Price = strings.TrimSpace(cfg.Price)
	cfg.Contact = strings.TrimSpace(cfg.Contact)
	cfg.ServerURL = strings.TrimSpace(cfg.ServerURL)
	return cfg, nil
}

func (c CommerceConfig) PurchaseReady(dir string) error {
	if !c.Enabled || c.Price == "" || c.Contact == "" {
		return errors.New("购买入口尚未开放。已有激活码可直接激活。")
	}
	p := filepath.Join(dir, "payment_qr.png")
	info, err := os.Stat(p)
	if err != nil || info.Size() > 16<<20 {
		return errors.New("购买入口尚未开放：收款码暂不可用。")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return errors.New("购买入口尚未开放：收款码暂不可用。")
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) == "251590ee73a63557bfe03894f02bb95824898f7b3ff5568b52ddf491940d5b9f" {
		return errors.New("购买入口尚未开放：收款码暂不可用。")
	}
	if _, err := png.DecodeConfig(bytes.NewReader(b)); err != nil {
		return errors.New("购买入口尚未开放：收款码暂不可用。")
	}
	return nil
}
