package admin

import (
	"bytes"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var web embed.FS

type HTTPOptions struct {
	PublicURL      string
	DownloadURL    string
	TrustedProxies []netip.Prefix
}
type quota struct {
	since time.Time
	count int
}
type handler struct {
	store       *Store
	origin      string
	downloadURL string
	secure      bool
	mu          sync.Mutex
	limits      map[string]quota
	images      chan struct{}
	proxies     []netip.Prefix
}

func NewHandler(store *Store, opts HTTPOptions) http.Handler {
	h := &handler{store: store, origin: strings.TrimRight(opts.PublicURL, "/"), downloadURL: opts.DownloadURL, secure: strings.HasPrefix(opts.PublicURL, "https://"), limits: map[string]quota{}, images: make(chan struct{}, 2), proxies: append([]netip.Prefix(nil), opts.TrustedProxies...)}
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin", http.StatusFound) })
	m.HandleFunc("GET /admin", h.page("admin.html"))
	if h.downloadURL != "" {
		m.HandleFunc("GET /download", h.download)
	}
	m.HandleFunc("GET /buy/{id}", h.page("buy.html"))
	m.HandleFunc("GET /static/{name}", h.asset)
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	m.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		cfg, err := store.Settings()
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, 200, cfg)
	})
	m.HandleFunc("GET /api/payment-qr", h.qr)
	m.HandleFunc("POST /api/orders", h.create)
	m.HandleFunc("GET /api/orders/{id}", h.order)
	m.HandleFunc("POST /api/orders/{id}/submit", h.submit)
	m.HandleFunc("POST /api/orders/{id}/evidence", h.evidenceUpload)
	m.HandleFunc("POST /api/orders/{id}/activated", h.activationReceipt)
	m.HandleFunc("POST /api/telemetry", h.telemetry)
	m.HandleFunc("POST /api/telemetry/error", h.telemetryError)
	m.HandleFunc("POST /api/admin/login", h.login)
	m.HandleFunc("GET /api/admin/me", h.admin(h.me))
	m.HandleFunc("POST /api/admin/logout", h.admin(h.logout))
	m.HandleFunc("POST /api/admin/password", h.admin(h.password))
	m.HandleFunc("GET /api/admin/orders", h.admin(h.orders))
	m.HandleFunc("GET /api/admin/analytics", h.admin(h.analytics))
	m.HandleFunc("GET /api/admin/users", h.admin(h.analyticsUsers))
	m.HandleFunc("POST /api/admin/orders/{id}/{action}", h.admin(h.decision))
	m.HandleFunc("GET /api/admin/orders/{id}/evidence", h.admin(h.evidence))
	m.HandleFunc("GET /api/admin/settings", h.admin(h.settings))
	m.HandleFunc("POST /api/admin/settings", h.admin(h.saveSettings))
	m.HandleFunc("POST /api/admin/payment-qr", h.admin(h.uploadQR))
	m.HandleFunc("GET /api/admin/payment-qr", h.admin(func(w http.ResponseWriter, r *http.Request) {
		h.sendImage(w, r, "SELECT qr,qr_mime FROM settings WHERE id=1")
	}))
	m.HandleFunc("GET /api/admin/audit", h.admin(h.audit))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				allowed := h.origin
				if allowed == "" {
					scheme := "http"
					if r.TLS != nil {
						scheme = "https"
					}
					allowed = scheme + "://" + r.Host
				}
				if origin != allowed {
					writeJSON(w, 403, map[string]string{"error": "请求来源不匹配"})
					return
				}
			}
		}
		m.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, 404, map[string]string{"error": "订单不存在或无权访问"})
		return
	}
	if _, ok := err.(interface{ Code() int }); ok {
		writeJSON(w, 500, map[string]string{"error": "数据暂时无法保存，请稍后再试"})
		return
	}
	writeJSON(w, 400, map[string]string{"error": safeError(err)})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeJSON(w, 415, map[string]string{"error": "需要 JSON 请求"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		writeJSON(w, 400, map[string]string{"error": "请求内容无效或过长"})
		return false
	}
	return true
}
func (h *handler) allowed(w http.ResponseWriter, r *http.Request, group string, limit int) bool {
	key := group + ":" + h.clientIP(r)
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.limits) > 4096 {
		for k, v := range h.limits {
			if now.Sub(v.since) >= time.Minute {
				delete(h.limits, k)
			}
		}
		if len(h.limits) > 4096 {
			writeJSON(w, 429, map[string]string{"error": "请求较多，请稍后重试"})
			return false
		}
	}
	v := h.limits[key]
	if now.Sub(v.since) >= time.Minute {
		v = quota{since: now}
	}
	v.count++
	h.limits[key] = v
	if v.count > limit {
		w.Header().Set("Retry-After", "60")
		writeJSON(w, 429, map[string]string{"error": "操作过于频繁，请一分钟后重试"})
		return false
	}
	return true
}
func (h *handler) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := web.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(b)
	}
}
func (h *handler) asset(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name != "style.css" && name != "admin.js" && name != "buy.js" {
		http.NotFound(w, r)
		return
	}
	b, err := web.ReadFile("web/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := "text/javascript; charset=utf-8"
	if strings.HasSuffix(name, ".css") {
		ct = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(b)
}
func bearer(r *http.Request) string {
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || len(parts[1]) != 64 {
		return ""
	}
	return parts[1]
}
func (h *handler) owned(w http.ResponseWriter, r *http.Request) (Order, bool) {
	o, err := h.store.OwnedOrder(r.PathValue("id"), bearer(r))
	if err != nil {
		fail(w, sql.ErrNoRows)
		return o, false
	}
	return o, true
}
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "order", 10) {
		return
	}
	var v struct {
		DeviceID string `json:"device_id"`
	}
	if !decode(w, r, &v) {
		return
	}
	o, token, err := h.store.CreateOrder(v.DeviceID)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"order": o, "token": token, "purchase_path": "/buy/" + url.PathEscape(o.ID) + "#" + token})
}
func (h *handler) order(w http.ResponseWriter, r *http.Request) {
	if o, ok := h.owned(w, r); ok {
		writeJSON(w, 200, o)
	}
}
func (h *handler) submit(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "submit", 20) {
		return
	}
	if _, ok := h.owned(w, r); !ok {
		return
	}
	var v Submission
	if !decode(w, r, &v) {
		return
	}
	if err := h.store.Submit(r.PathValue("id"), bearer(r), v); err != nil {
		fail(w, err)
		return
	}
	h.order(w, r)
}
func (h *handler) readImage(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	select {
	case h.images <- struct{}{}:
		defer func() { <-h.images }()
	default:
		return nil, "", errors.New("正在处理其他图片，请稍后重试")
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		return nil, "", errors.New("图片不能超过 4 MB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || (format != "png" && format != "jpeg") || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 8000000 {
		return nil, "", errors.New("请上传有效的 PNG/JPG 图片，最多 800 万像素")
	}
	if _, _, err = image.Decode(bytes.NewReader(b)); err != nil {
		return nil, "", errors.New("图片内容不完整")
	}
	ct := "image/png"
	if format == "jpeg" {
		ct = "image/jpeg"
	}
	return b, ct, nil
}
func (h *handler) evidenceUpload(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "image", 12) {
		return
	}
	o, ok := h.owned(w, r)
	if !ok {
		return
	}
	if o.Status == "approved" {
		fail(w, errors.New("已开通的订单不能更改凭证"))
		return
	}
	b, ct, err := h.readImage(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := h.store.db.Exec("UPDATE orders SET evidence=?,evidence_mime=?,evidence_hash=?,updated_at=?,revision=revision+1 WHERE id=? AND token_hash=? AND status<>'approved'", b, ct, tokenHash(string(b)), stamp(), o.ID, tokenHash(bearer(r)))
	if err != nil {
		fail(w, err)
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		fail(w, errors.New("订单状态已更新，请刷新后重试"))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *handler) qr(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.Settings()
	if err != nil {
		fail(w, err)
		return
	}
	if !cfg.Enabled || !cfg.QRAvailable {
		http.NotFound(w, r)
		return
	}
	h.sendImage(w, r, "SELECT qr,qr_mime FROM settings WHERE id=1")
}
func (h *handler) sendImage(w http.ResponseWriter, r *http.Request, q string, args ...any) {
	var b []byte
	var ct string
	if err := h.store.db.QueryRow(q, args...).Scan(&b, &ct); err != nil || len(b) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(b)
}

func (h *handler) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("workmate_admin")
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "请先登录后台"})
			return
		}
		csrf, err := h.store.Session(cookie.Value)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "登录已过期，请重新登录"})
			return
		}
		if r.Method != "GET" {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
				writeJSON(w, 403, map[string]string{"error": "会话校验失败，请刷新后重试"})
				return
			}
			if !h.allowed(w, r, "admin", 120) {
				return
			}
		}
		next(w, r)
	}
}
func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "login", 5) {
		return
	}
	var v struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	if v.Username != "admin" || len(v.Password) > 72 {
		writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
		return
	}
	token, csrf, err := h.store.Login(v.Password)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "账号或密码错误"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "workmate_admin", Value: token, Path: "/", HttpOnly: true, Secure: h.secure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
	writeJSON(w, 200, map[string]string{"csrf": csrf, "username": "admin"})
}
func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("workmate_admin")
	csrf, err := h.store.Session(c.Value)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"username": "admin", "csrf": csrf})
}
func (h *handler) clearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "workmate_admin", Value: "", Path: "/", HttpOnly: true, Secure: h.secure || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	c, _ := r.Cookie("workmate_admin")
	if err := h.store.Logout(c.Value); err != nil {
		fail(w, err)
		return
	}
	h.clearCookie(w, r)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *handler) password(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Old string `json:"old_password"`
		New string `json:"new_password"`
	}
	if !decode(w, r, &v) {
		return
	}
	if err := h.store.ChangePassword(v.Old, v.New); err != nil {
		fail(w, err)
		return
	}
	h.clearCookie(w, r)
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *handler) orders(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" && status != "created" && status != "pending" && status != "approved" && status != "rejected" {
		fail(w, errors.New("订单状态无效"))
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 || offset > 10000000 || len(search) > 240 {
		fail(w, errors.New("查询条件无效"))
		return
	}
	orders, total, err := h.store.List(status, search, offset)
	if err != nil {
		fail(w, err)
		return
	}
	summary, err := h.store.Summary()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"orders": orders, "total": total, "summary": summary})
}
func (h *handler) decision(w http.ResponseWriter, r *http.Request) {
	var d Decision
	if !decode(w, r, &d) {
		return
	}
	o, err := h.store.Decide(r.PathValue("id"), r.PathValue("action"), d)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, o)
}
func (h *handler) evidence(w http.ResponseWriter, r *http.Request) {
	h.sendImage(w, r, "SELECT evidence,evidence_mime FROM orders WHERE id=?", r.PathValue("id"))
}
func (h *handler) settings(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.Settings()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"settings": cfg, "public_key": h.store.PublicKey()})
}
func (h *handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	var cfg Settings
	if !decode(w, r, &cfg) {
		return
	}
	if err := h.store.UpdateSettings(cfg); err != nil {
		fail(w, err)
		return
	}
	h.settings(w, r)
}
func (h *handler) uploadQR(w http.ResponseWriter, r *http.Request) {
	b, ct, err := h.readImage(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	if tokenHash(string(b)) == "251590ee73a63557bfe03894f02bb95824898f7b3ff5568b52ddf491940d5b9f" {
		fail(w, errors.New("请上传自己的收款码，不能使用项目占位图片"))
		return
	}
	if _, err = h.store.db.Exec("UPDATE settings SET qr=?,qr_mime=? WHERE id=1", b, ct); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (h *handler) audit(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.Audit()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, items)
}

func (h *handler) telemetry(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "telemetry", 60) {
		return
	}
	var v TelemetryReport
	if !decode(w, r, &v) {
		return
	}
	if err := h.store.RecordTelemetry(v); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) telemetryError(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "telemetry_error", 20) {
		return
	}
	var v ErrorReport
	if !decode(w, r, &v) {
		return
	}
	if err := h.store.RecordError(v); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) activationReceipt(w http.ResponseWriter, r *http.Request) {
	if !h.allowed(w, r, "activation", 20) {
		return
	}
	if err := h.store.AckActivation(r.PathValue("id"), bearer(r)); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (h *handler) download(w http.ResponseWriter, r *http.Request) {
	if err := h.store.RecordDownloadClick(); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, h.downloadURL, http.StatusFound)
}

func (h *handler) analytics(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.AnalyticsOverview()
	if err != nil {
		fail(w, err)
		return
	}
	if h.downloadURL != "" && v.DownloadClicks == nil {
		zero := 0
		v.DownloadClicks = &zero
	}
	writeJSON(w, 200, v)
}

func (h *handler) analyticsUsers(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if r.URL.Query().Get("offset") == "" {
		offset = 0
		err = nil
	}
	if err != nil {
		fail(w, errors.New("查询条件无效"))
		return
	}
	users, total, err := h.store.AnalyticsUsers(r.URL.Query().Get("search"), offset)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"users": users, "total": total})
}
