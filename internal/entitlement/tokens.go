package entitlement

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type License struct {
	Version   int    `json:"v"`
	LicenseID string `json:"license_id"`
	Edition   string `json:"edition"`
	DeviceID  string `json:"device_id"`
	IssuedAt  string `json:"issued_at"`
	Customer  string `json:"customer,omitempty"`
}

type TrialPolicy struct {
	Version  int    `json:"v"`
	Kind     string `json:"kind"`
	Hours    int    `json:"hours"`
	IssuedAt string `json:"issued_at"`
}

func Sign(value any, key ed25519.PrivateKey) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", errors.New("invalid signing key")
	}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, b)), nil
}

func Verify(token, publicKey string, value any) error {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || len(token) > 8192 {
		return errors.New("授权内容格式不正确")
	}
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("尚未配置后台授权公钥")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("授权内容损坏")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !ed25519.Verify(pub, payload, sig) {
		return errors.New("授权签名无效")
	}
	if json.Unmarshal(payload, value) != nil {
		return errors.New("授权内容无效")
	}
	return nil
}

func VerifyTrial(token, publicKey string) (TrialPolicy, error) {
	var p TrialPolicy
	if err := Verify(token, publicKey, &p); err != nil {
		return p, err
	}
	if p.Version != 1 || p.Kind != "trial-policy" || p.Hours < 1 || p.Hours > 8760 {
		return p, errors.New("试用策略无效")
	}
	if _, err := time.Parse(time.RFC3339, p.IssuedAt); err != nil {
		return p, errors.New("试用策略时间无效")
	}
	return p, nil
}
