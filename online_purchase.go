package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Set at build time. Editable commerce.json cannot introduce trusted keys.
var onlineLicensePublicKeyB64 string

type onlineConfig struct {
	Enabled     bool   `json:"enabled"`
	PriceCents  int    `json:"price_cents"`
	Contact     string `json:"contact"`
	TrialHours  int    `json:"trial_hours"`
	QRAvailable bool   `json:"qr_available"`
	TrialPolicy string `json:"trial_policy"`
}
type purchaseTicket struct {
	ServerURL string `json:"server_url"`
	OrderID   string `json:"order_id"`
	DeviceID  string `json:"device_id"`
	Token     string `json:"token"`
}
type onlineOrder struct {
	ID          string `json:"id"`
	DeviceID    string `json:"device_id"`
	Status      string `json:"status"`
	AmountCents int    `json:"amount_cents"`
	AdminNote   string `json:"admin_note"`
	LicenseCode string `json:"license_code"`
}
type purchaseClient struct {
	base, device, dataDir string
	http                  *http.Client
	ticket                *purchaseTicket
}

func validateServerURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("后台地址需要填写完整的 HTTPS 域名")
	}
	ip := net.ParseIP(u.Hostname())
	local := strings.EqualFold(u.Hostname(), "localhost") || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New("后台地址必须使用 HTTPS，本机测试可使用 http://127.0.0.1:8090")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func newPurchaseClient(raw, device, dataDir string) (*purchaseClient, error) {
	base, err := validateServerURL(raw)
	if err != nil {
		return nil, err
	}
	c := &purchaseClient{base: base, device: device, dataDir: dataDir, http: &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("后台地址发生跳转，请检查配置")
	}}}
	if b, e := os.ReadFile(filepath.Join(dataDir, "online-order.json")); e == nil {
		var t purchaseTicket
		if len(b) <= 8192 && json.Unmarshal(b, &t) == nil && t.ServerURL == base && t.DeviceID == device && validTicket(t) {
			c.ticket = &t
		}
	}
	return c, nil
}
func validTicket(t purchaseTicket) bool {
	b, err := hex.DecodeString(t.Token)
	return err == nil && len(b) == 32 && len(t.OrderID) >= 8 && len(t.OrderID) <= 64 && !strings.ContainsAny(t.OrderID, "/\\?# \t\r\n")
}
func (c *purchaseClient) saveTicket(t purchaseTicket) error {
	if t.ServerURL != c.base || t.DeviceID != c.device || !validTicket(t) {
		return errors.New("订单访问凭证无效")
	}
	c.ticket = &t // Keep the order available if saving temporarily fails.
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	path := filepath.Join(c.dataDir, "online-order.json")
	f, err := os.CreateTemp(c.dataDir, ".online-order-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func (c *purchaseClient) request(method, path, token string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	r, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(r)
	if err != nil {
		return errors.New("暂时无法连接购买后台，请检查网络后重试")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, (64<<10)+1))
	if err != nil || len(b) > 64<<10 {
		return errors.New("购买后台返回的内容无效")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var v struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &v) == nil && v.Error != "" && len(v.Error) <= 600 {
			return errors.New(v.Error)
		}
		return errors.New("购买后台暂时无法处理请求，请稍后再试")
	}
	if json.Unmarshal(b, out) != nil {
		return errors.New("购买后台返回的内容无效")
	}
	return nil
}
func (c *purchaseClient) config() (onlineConfig, error) {
	var cfg onlineConfig
	err := c.request("GET", "/api/config", "", nil, &cfg)
	return cfg, err
}
func (c *purchaseClient) create() (purchaseTicket, onlineOrder, error) {
	var response struct {
		Order onlineOrder `json:"order"`
		Token string      `json:"token"`
	}
	err := c.request("POST", "/api/orders", "", map[string]string{"device_id": c.device}, &response)
	t := purchaseTicket{ServerURL: c.base, OrderID: response.Order.ID, DeviceID: c.device, Token: response.Token}
	if err == nil && (!validTicket(t) || response.Order.DeviceID != c.device || response.Order.Status != "created" || response.Order.AmountCents < 1) {
		err = errors.New("购买后台返回的订单无效")
	}
	return t, response.Order, err
}
func (c *purchaseClient) status(t purchaseTicket) (onlineOrder, error) {
	var order onlineOrder
	if !validTicket(t) || t.ServerURL != c.base || t.DeviceID != c.device {
		return order, errors.New("订单访问凭证无效")
	}
	err := c.request("GET", "/api/orders/"+url.PathEscape(t.OrderID), t.Token, nil, &order)
	if err == nil && (order.ID != t.OrderID || order.DeviceID != c.device || (order.Status != "created" && order.Status != "pending" && order.Status != "approved" && order.Status != "rejected")) {
		err = errors.New("购买后台返回的订单无效")
	}
	return order, err
}
func (c *purchaseClient) purchaseURL() string {
	if c.ticket == nil {
		return ""
	}
	return c.base + "/buy/" + url.PathEscape(c.ticket.OrderID) + "#" + c.ticket.Token
}
