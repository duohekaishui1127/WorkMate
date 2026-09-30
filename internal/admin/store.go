package admin

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
	"workmate/internal/entitlement"
)

type Settings struct {
	Enabled     bool   `json:"enabled"`
	PriceCents  int    `json:"price_cents"`
	Contact     string `json:"contact"`
	TrialHours  int    `json:"trial_hours"`
	QRAvailable bool   `json:"qr_available"`
	TrialPolicy string `json:"trial_policy,omitempty"`
}

type Order struct {
	ID               string `json:"id"`
	DeviceID         string `json:"device_id"`
	Status           string `json:"status"`
	AmountCents      int    `json:"amount_cents"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	Contact          string `json:"contact"`
	PaymentMethod    string `json:"payment_method"`
	PaymentReference string `json:"payment_reference"`
	CustomerNote     string `json:"customer_note"`
	AdminNote        string `json:"admin_note"`
	ReceiptReference string `json:"receipt_reference"`
	HasEvidence      bool   `json:"has_evidence"`
	EvidenceUses     int    `json:"evidence_uses"`
	Revision         int    `json:"revision"`
	LicenseCode      string `json:"license_code,omitempty"`
}

type Store struct {
	db  *sql.DB
	key ed25519.PrivateKey
	dir string
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func tokenHash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
func stamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "license-private.key")
	b, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		if info, e := os.Stat(filepath.Join(dir, "workmate.sqlite")); e == nil && info.Size() > 0 {
			return nil, errors.New("已有授权数据库但私钥缺失，请恢复原始私钥；不能自动更换密钥")
		}
		_, key, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return nil, e
		}
		b = []byte(base64.StdEncoding.EncodeToString(key.Seed()) + "\n")
		if e = writeExclusive(keyPath, b, 0600); e != nil {
			return nil, e
		}
	} else if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("后台私钥无效，请恢复原始密钥文件")
	}
	key := ed25519.NewKeyFromSeed(seed)
	public := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	pubPath := filepath.Join(dir, "license-public.key")
	if old, e := os.ReadFile(pubPath); e == nil {
		if strings.TrimSpace(string(old)) != public {
			return nil, errors.New("后台公私钥不匹配，请恢复原始密钥文件")
		}
	} else if os.IsNotExist(e) {
		if e = writeExclusive(pubPath, []byte(public+"\n"), 0644); e != nil {
			return nil, e
		}
	} else {
		return nil, e
	}
	dbPath := filepath.Join(dir, "workmate.sqlite")
	f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, key: key, dir: dir}
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS settings (id INTEGER PRIMARY KEY CHECK(id=1), body TEXT NOT NULL, qr BLOB, qr_mime TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS administrators (id INTEGER PRIMARY KEY CHECK(id=1), password_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, csrf TEXT NOT NULL, expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS orders (
 id TEXT PRIMARY KEY, device_id TEXT NOT NULL, token_hash TEXT NOT NULL, status TEXT NOT NULL CHECK(status IN ('created','pending','approved','rejected')),
 amount_cents INTEGER NOT NULL CHECK(amount_cents>0), created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 contact TEXT NOT NULL DEFAULT '', payment_method TEXT NOT NULL DEFAULT '', payment_reference TEXT NOT NULL DEFAULT '', customer_note TEXT NOT NULL DEFAULT '',
 admin_note TEXT NOT NULL DEFAULT '', receipt_reference TEXT NOT NULL DEFAULT '', evidence BLOB, evidence_mime TEXT NOT NULL DEFAULT '', evidence_hash TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL DEFAULT 1, license_code TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS orders_status ON orders(status,created_at);
CREATE INDEX IF NOT EXISTS orders_device ON orders(device_id);
CREATE INDEX IF NOT EXISTS orders_proof ON orders(evidence_hash);
CREATE UNIQUE INDEX IF NOT EXISTS receipts_once ON orders(receipt_reference) WHERE status='approved';
CREATE TABLE IF NOT EXISTS payment_checkouts (
 order_id TEXT PRIMARY KEY REFERENCES orders(id),
 provider TEXT NOT NULL,
 provider_order_id TEXT NOT NULL,
 UNIQUE(provider,provider_order_id));
CREATE TABLE IF NOT EXISTS audit (id INTEGER PRIMARY KEY AUTOINCREMENT, occurred_at TEXT NOT NULL, action TEXT NOT NULL, order_id TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '');`)
	if err != nil {
		db.Close()
		return nil, err
	}
	defaults, _ := json.Marshal(Settings{PriceCents: 1990, TrialHours: 24})
	if _, err = db.Exec("INSERT OR IGNORE INTO settings(id,body) VALUES(1,?)", string(defaults)); err != nil {
		db.Close()
		return nil, err
	}
	var count int
	if err = db.QueryRow("SELECT count(*) FROM administrators").Scan(&count); err != nil {
		db.Close()
		return nil, err
	}
	if count == 0 {
		password, e := randomToken(18)
		if e != nil {
			db.Close()
			return nil, e
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
		if e != nil {
			db.Close()
			return nil, e
		}
		// The initial password is never printed to logs or committed to the repository.
		if e = os.WriteFile(filepath.Join(dir, "initial-password.txt"), []byte(password+"\n"), 0600); e != nil {
			db.Close()
			return nil, e
		}
		if _, e = db.Exec("INSERT INTO administrators(id,password_hash) VALUES(1,?)", string(hash)); e != nil {
			db.Close()
			return nil, e
		}
	}
	return s, nil
}

func writeExclusive(path string, b []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) PublicKey() string {
	return base64.StdEncoding.EncodeToString(s.key.Public().(ed25519.PublicKey))
}

func (s *Store) Settings() (Settings, error) {
	var cfg Settings
	var body string
	var available bool
	err := s.db.QueryRow("SELECT body,COALESCE(length(qr)>0,0) FROM settings WHERE id=1").Scan(&body, &available)
	if err != nil {
		return cfg, err
	}
	if err = json.Unmarshal([]byte(body), &cfg); err != nil {
		return cfg, err
	}
	cfg.QRAvailable = available
	cfg.TrialPolicy, err = entitlement.Sign(entitlement.TrialPolicy{Version: 1, Kind: "trial-policy", Hours: cfg.TrialHours, IssuedAt: time.Now().UTC().Format(time.RFC3339Nano)}, s.key)
	return cfg, err
}

func (s *Store) UpdateSettings(cfg Settings) error {
	cfg.Contact = strings.TrimSpace(cfg.Contact)
	if cfg.PriceCents < 1 || cfg.PriceCents > 100000000 || cfg.TrialHours < 1 || cfg.TrialHours > 8760 || len(cfg.Contact) > 240 {
		return errors.New("价格需为 0.01–1000000 元，试用时长需为 1–8760 小时")
	}
	current, err := s.Settings()
	if err != nil {
		return err
	}
	if cfg.Enabled && (!current.QRAvailable || cfg.Contact == "") {
		return errors.New("请先上传收款码并填写联系方式，再开放购买")
	}
	cfg.QRAvailable = false
	cfg.TrialPolicy = ""
	b, _ := json.Marshal(cfg)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE settings SET body=? WHERE id=1", string(b)); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action,note) VALUES(?, 'settings', ?)", stamp(), string(b)); err != nil {
		return err
	}
	return tx.Commit()
}

const orderColumns = `id,device_id,status,amount_cents,created_at,updated_at,contact,payment_method,payment_reference,customer_note,admin_note,receipt_reference,COALESCE(length(evidence)>0,0),revision,license_code`

type scanner interface{ Scan(...any) error }

func scanOrder(row scanner) (Order, error) {
	var o Order
	err := row.Scan(&o.ID, &o.DeviceID, &o.Status, &o.AmountCents, &o.CreatedAt, &o.UpdatedAt, &o.Contact, &o.PaymentMethod, &o.PaymentReference, &o.CustomerNote, &o.AdminNote, &o.ReceiptReference, &o.HasEvidence, &o.Revision, &o.LicenseCode)
	return o, err
}
func (s *Store) Order(id string) (Order, error) {
	return scanOrder(s.db.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=?", id))
}
func (s *Store) OwnedOrder(id, token string) (Order, error) {
	return scanOrder(s.db.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=? AND token_hash=?", id, tokenHash(token)))
}

func (s *Store) CreateOrder(device string) (Order, string, error) {
	var empty Order
	device = strings.ToLower(strings.TrimSpace(device))
	b, err := hex.DecodeString(device)
	if err != nil || len(b) != 32 {
		return empty, "", errors.New("设备码必须为完整的 64 位设备码")
	}
	cfg, err := s.Settings()
	if err != nil {
		return empty, "", err
	}
	if !cfg.Enabled || !cfg.QRAvailable || cfg.Contact == "" {
		return empty, "", errors.New("购买入口尚未开放")
	}
	random, err := randomToken(8)
	if err != nil {
		return empty, "", err
	}
	token, err := randomToken(32)
	if err != nil {
		return empty, "", err
	}
	now := stamp()
	o := Order{ID: "WM" + time.Now().UTC().Format("20060102") + "-" + strings.ToUpper(random), DeviceID: device, Status: "created", AmountCents: cfg.PriceCents, CreatedAt: now, UpdatedAt: now, Revision: 1}
	tx, err := s.db.Begin()
	if err != nil {
		return empty, "", err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow("SELECT count(*) FROM orders WHERE device_id=? AND status IN ('created','pending','rejected')", device).Scan(&count); err != nil {
		return empty, "", err
	}
	if count >= 5 {
		return empty, "", errors.New("这台电脑已有多个未完成订单，请先处理已有订单")
	}
	if _, err = tx.Exec("INSERT INTO orders(id,device_id,token_hash,status,amount_cents,created_at,updated_at) VALUES(?,?,?,'created',?,?,?)", o.ID, device, tokenHash(token), o.AmountCents, now, now); err != nil {
		return empty, "", err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action,order_id) VALUES(?,'created',?)", now, o.ID); err != nil {
		return empty, "", err
	}
	return o, token, tx.Commit()
}

type Submission struct {
	Contact   string `json:"contact"`
	Method    string `json:"payment_method"`
	Reference string `json:"payment_reference"`
	Note      string `json:"customer_note"`
}

func (s *Store) Submit(id, token string, v Submission) error {
	v.Contact = strings.TrimSpace(v.Contact)
	v.Reference = strings.TrimSpace(v.Reference)
	v.Note = strings.TrimSpace(v.Note)
	v.Method = strings.TrimSpace(v.Method)
	if v.Method == "" {
		v.Method = "other"
	}
	if len(v.Contact) > 240 || len(v.Reference) > 160 || len(v.Note) > 1000 || (v.Method != "wechat" && v.Method != "alipay" && v.Method != "other") {
		return errors.New("付款方式无效或填写内容过长，请检查后重新提交")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	o, err := scanOrder(tx.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=? AND token_hash=?", id, tokenHash(token)))
	if err != nil {
		return err
	}
	if o.Status == "approved" {
		return errors.New("订单已开通，不能修改付款信息")
	}
	if !o.HasEvidence && len(v.Reference) < 3 {
		return errors.New("请上传付款凭证或填写付款交易单号")
	}
	if _, err = tx.Exec("UPDATE orders SET contact=?,payment_method=?,payment_reference=?,customer_note=?,status='pending',admin_note='',updated_at=?,revision=revision+1 WHERE id=?", v.Contact, v.Method, v.Reference, v.Note, stamp(), id); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action,order_id) VALUES(?,'submitted',?)", stamp(), id); err != nil {
		return err
	}
	return tx.Commit()
}

type Decision struct {
	Revision      int    `json:"revision"`
	Confirmed     bool   `json:"confirmed"`
	ReceivedCents int    `json:"received_cents"`
	Receipt       string `json:"receipt_reference"`
	Note          string `json:"note"`
}

func (s *Store) Decide(id, action string, d Decision) (Order, error) {
	var empty Order
	d.Receipt = strings.TrimSpace(d.Receipt)
	d.Note = strings.TrimSpace(d.Note)
	if len(d.Note) > 1000 || len(d.Receipt) > 160 {
		return empty, errors.New("备注或交易单号过长")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	o, err := scanOrder(tx.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=?", id))
	if err != nil {
		return empty, err
	}
	if o.Status == "approved" && action == "approve" {
		return o, nil
	}
	if o.Status != "pending" {
		return empty, errors.New("只能处理待核实的付款订单")
	}
	if o.Revision != d.Revision {
		return empty, errors.New("订单信息已更新，请刷新后重新核实")
	}
	if action == "approve" {
		if !d.Confirmed || d.ReceivedCents != o.AmountCents || len(d.Receipt) < 3 {
			return empty, errors.New("请确认实际到账，填写对应金额及实际到账交易单号")
		}
		var used int
		if err = tx.QueryRow("SELECT count(*) FROM orders WHERE status='approved' AND receipt_reference=?", d.Receipt).Scan(&used); err != nil {
			return empty, err
		}
		if used > 0 {
			return empty, errors.New("该到账交易单号已经用于其他订单，请勿重复开通")
		}
		code, e := entitlement.Sign(entitlement.License{Version: 1, LicenseID: o.ID, Edition: "pro", DeviceID: o.DeviceID, IssuedAt: time.Now().UTC().Format(time.RFC3339), Customer: o.Contact}, s.key)
		if e != nil {
			return empty, e
		}
		_, err = tx.Exec("UPDATE orders SET status='approved',license_code=?,admin_note=?,receipt_reference=?,updated_at=?,revision=revision+1 WHERE id=?", code, d.Note, d.Receipt, stamp(), id)
	} else if action == "reject" {
		if d.Note == "" {
			return empty, errors.New("请填写驳回原因，方便用户补充信息")
		}
		_, err = tx.Exec("UPDATE orders SET status='rejected',admin_note=?,updated_at=?,revision=revision+1 WHERE id=?", d.Note, stamp(), id)
	} else {
		return empty, errors.New("未知操作")
	}
	if err != nil {
		return empty, err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action,order_id,note) VALUES(?,?,?,?)", stamp(), action, id, d.Note); err != nil {
		return empty, err
	}
	o, err = scanOrder(tx.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=?", id))
	if err != nil {
		return empty, err
	}
	return o, tx.Commit()
}

func (s *Store) List(status, search string, offset int) ([]Order, int, error) {
	where := " WHERE (?='' OR status=?) AND (?='' OR instr(id,?)>0 OR instr(device_id,?)>0 OR instr(contact,?)>0 OR instr(payment_reference,?)>0 OR instr(receipt_reference,?)>0)"
	args := []any{status, status, search, search, search, search, search, search}
	var total int
	if err := s.db.QueryRow("SELECT count(*) FROM orders"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.Query("SELECT "+orderColumns+" FROM orders"+where+" ORDER BY created_at DESC,id DESC LIMIT 50 OFFSET ?", append(args, offset)...)
	if err != nil {
		return nil, 0, err
	}
	var orders = []Order{}
	for rows.Next() {
		o, e := scanOrder(rows)
		if e != nil {
			rows.Close()
			return nil, 0, e
		}
		orders = append(orders, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, err
	}
	for i := range orders {
		if orders[i].HasEvidence {
			if err = s.db.QueryRow("SELECT count(*) FROM orders WHERE evidence_hash<>'' AND evidence_hash=(SELECT evidence_hash FROM orders WHERE id=?)", orders[i].ID).Scan(&orders[i].EvidenceUses); err != nil {
				return nil, 0, err
			}
		}
	}
	return orders, total, nil
}

func (s *Store) Summary() (map[string]int, error) {
	result := map[string]int{"created": 0, "pending": 0, "approved": 0, "rejected": 0}
	rows, err := s.db.Query("SELECT status,count(*) FROM orders GROUP BY status")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int
		if err = rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		result[key] = count
	}
	return result, rows.Err()
}

func (s *Store) Login(password string) (string, string, error) {
	var hash string
	if err := s.db.QueryRow("SELECT password_hash FROM administrators WHERE id=1").Scan(&hash); err != nil {
		return "", "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", errors.New("账号或密码错误")
	}
	token, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	_, err = s.db.Exec("DELETE FROM sessions WHERE expires<?", time.Now().Unix())
	if err != nil {
		return "", "", err
	}
	_, err = s.db.Exec("INSERT INTO sessions(token_hash,csrf,expires) VALUES(?,?,?)", tokenHash(token), csrf, time.Now().Add(12*time.Hour).Unix())
	return token, csrf, err
}
func (s *Store) Session(token string) (string, error) {
	var csrf string
	err := s.db.QueryRow("SELECT csrf FROM sessions WHERE token_hash=? AND expires>?", tokenHash(token), time.Now().Unix()).Scan(&csrf)
	return csrf, err
}
func (s *Store) Logout(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash=?", tokenHash(token))
	return err
}
func (s *Store) ChangePassword(old, new string) error {
	if len(new) < 12 || len(new) > 72 {
		return errors.New("新密码需为 12–72 字节")
	}
	var hash string
	if err := s.db.QueryRow("SELECT password_hash FROM administrators WHERE id=1").Scan(&hash); err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(old)) != nil {
		return errors.New("当前密码不正确")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(new), 12)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE administrators SET password_hash=? WHERE id=1", string(b)); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM sessions"); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action) VALUES(?,'password_changed')", stamp()); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(s.dir, "initial-password.txt"))
	return nil
}

func (s *Store) Audit() ([]map[string]string, error) {
	rows, err := s.db.Query("SELECT occurred_at,action,order_id,note FROM audit ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]string{}
	for rows.Next() {
		var at, action, id, note string
		if err = rows.Scan(&at, &action, &id, &note); err != nil {
			return nil, err
		}
		items = append(items, map[string]string{"at": at, "action": action, "order_id": id, "note": note})
	}
	return items, rows.Err()
}

func safeError(err error) string {
	if errors.Is(err, sql.ErrNoRows) {
		return "订单不存在或无权访问"
	}
	return fmt.Sprint(err)
}
