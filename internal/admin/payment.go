package admin

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"workmate/internal/entitlement"
)

// verifiedPayment must only be constructed after a payment provider's signature,
// merchant identity, payment state and transaction details have been verified.
// Neither a buyer's screenshot nor a buyer-supplied transaction number qualifies.
type verifiedPayment struct {
	Provider        string
	ProviderOrderID string
	TransactionID   string
	AmountCents     int
	Currency        string
}

func validPaymentID(value string, max int) bool {
	if len(value) < 3 || len(value) > max {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func validPaymentProvider(provider string) bool {
	return provider == "wechat" || provider == "alipay"
}

// registerCheckout ties a provider-created checkout to one existing order. A
// static personal collection QR never calls this method and cannot auto-approve.
func (s *Store) registerCheckout(orderID, provider, providerOrderID string) error {
	if !validPaymentProvider(provider) || !validPaymentID(providerOrderID, 64) {
		return errors.New("支付渠道或支付订单号无效")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err = tx.QueryRow("SELECT status FROM orders WHERE id=?", orderID).Scan(&status); err != nil {
		return err
	}
	if status != "created" {
		return errors.New("只有未处理订单可以创建支付订单")
	}
	var oldProvider, oldID string
	err = tx.QueryRow("SELECT provider,provider_order_id FROM payment_checkouts WHERE order_id=?", orderID).Scan(&oldProvider, &oldID)
	if err == nil {
		if oldProvider == provider && oldID == providerOrderID {
			return nil
		}
		return errors.New("订单已绑定另一笔支付订单")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err = tx.Exec("INSERT INTO payment_checkouts(order_id,provider,provider_order_id) VALUES(?,?,?)", orderID, provider, providerOrderID); err != nil {
		return err
	}
	return tx.Commit()
}

// approveVerifiedPayment signs a device-bound Pro license exactly once for a
// registered checkout. The caller must authenticate and verify the provider
// notification before calling this method.
func (s *Store) approveVerifiedPayment(v verifiedPayment) (Order, error) {
	var empty Order
	if !validPaymentProvider(v.Provider) || !validPaymentID(v.ProviderOrderID, 64) || !validPaymentID(v.TransactionID, 120) || v.AmountCents < 1 || v.Currency != "CNY" {
		return empty, errors.New("支付通知内容无效")
	}
	receipt := v.Provider + ":" + v.TransactionID
	tx, err := s.db.Begin()
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	var orderID string
	if err = tx.QueryRow("SELECT order_id FROM payment_checkouts WHERE provider=? AND provider_order_id=?", v.Provider, v.ProviderOrderID).Scan(&orderID); err != nil {
		return empty, err
	}
	o, err := scanOrder(tx.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=?", orderID))
	if err != nil {
		return empty, err
	}
	if o.AmountCents != v.AmountCents {
		return empty, errors.New("支付金额与订单不一致")
	}
	if o.Status == "approved" {
		if o.ReceiptReference == receipt {
			return o, nil
		}
		return empty, errors.New("订单已由另一笔付款开通")
	}
	code, err := entitlement.Sign(entitlement.License{
		Version: 1, LicenseID: o.ID, Edition: "pro", DeviceID: o.DeviceID,
		IssuedAt: time.Now().UTC().Format(time.RFC3339), Customer: o.Contact,
	}, s.key)
	if err != nil {
		return empty, err
	}
	if _, err = tx.Exec("UPDATE orders SET status='approved',license_code=?,payment_method=?,admin_note='',receipt_reference=?,updated_at=?,revision=revision+1 WHERE id=?", code, v.Provider, receipt, stamp(), o.ID); err != nil {
		return empty, err
	}
	if _, err = tx.Exec("INSERT INTO audit(occurred_at,action,order_id,note) VALUES(?,'auto_approve',?,?)", stamp(), o.ID, strings.ToUpper(v.Provider)); err != nil {
		return empty, err
	}
	o, err = scanOrder(tx.QueryRow("SELECT "+orderColumns+" FROM orders WHERE id=?", o.ID))
	if err != nil {
		return empty, err
	}
	return o, tx.Commit()
}
