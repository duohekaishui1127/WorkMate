package admin

import (
	"strings"
	"testing"

	"workmate/internal/entitlement"
)

func TestVerifiedPaymentApprovesOnlyMatchingCheckoutOnce(t *testing.T) {
	f := fixture(t)
	f.ready(t)
	order, _, err := f.store.CreateOrder(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	payment := verifiedPayment{
		Provider: "wechat", ProviderOrderID: order.ID,
		TransactionID: "WECHAT-TRANSACTION-001", AmountCents: order.AmountCents, Currency: "CNY",
	}
	if _, err := f.store.approveVerifiedPayment(payment); err == nil {
		t.Fatal("unregistered checkout granted Pro")
	}
	if err := f.store.registerCheckout(order.ID, "wechat", order.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.store.registerCheckout(order.ID, "wechat", order.ID); err != nil {
		t.Fatal("same checkout registration should be idempotent", err)
	}
	if err := f.store.registerCheckout(order.ID, "wechat", "DIFFERENT-ORDER"); err == nil {
		t.Fatal("checkout was rebound to another payment")
	}
	for _, bad := range []verifiedPayment{
		{Provider: "wechat", ProviderOrderID: order.ID, TransactionID: payment.TransactionID, AmountCents: 1, Currency: "CNY"},
		{Provider: "wechat", ProviderOrderID: order.ID, TransactionID: payment.TransactionID, AmountCents: order.AmountCents, Currency: "USD"},
		{Provider: "alipay", ProviderOrderID: order.ID, TransactionID: payment.TransactionID, AmountCents: order.AmountCents, Currency: "CNY"},
		{Provider: "wechat", ProviderOrderID: "WRONG-ORDER", TransactionID: payment.TransactionID, AmountCents: order.AmountCents, Currency: "CNY"},
	} {
		if _, err := f.store.approveVerifiedPayment(bad); err == nil {
			t.Fatal("unmatched payment granted Pro", bad)
		}
	}
	still, err := f.store.Order(order.ID)
	if err != nil || still.Status != "created" || still.LicenseCode != "" {
		t.Fatal("invalid payment changed the order", err)
	}
	approved, err := f.store.approveVerifiedPayment(payment)
	if err != nil || approved.Status != "approved" {
		t.Fatal("verified payment did not approve", err)
	}
	var license entitlement.License
	if err := entitlement.Verify(approved.LicenseCode, f.store.PublicKey(), &license); err != nil || license.LicenseID != order.ID || license.DeviceID != order.DeviceID {
		t.Fatal("automatic license is invalid or bound to wrong device", err)
	}
	again, err := f.store.approveVerifiedPayment(payment)
	if err != nil || again.LicenseCode != approved.LicenseCode {
		t.Fatal("repeat notification changed the license", err)
	}
	var count int
	if err := f.store.db.QueryRow("SELECT count(*) FROM audit WHERE action='auto_approve' AND order_id=?", order.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("payment notification approved more than once", count, err)
	}
	other, _, err := f.store.CreateOrder(strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.registerCheckout(other.ID, "wechat", order.ID); err == nil {
		t.Fatal("one provider checkout was assigned to two orders")
	}
	if err := f.store.registerCheckout(other.ID, "wechat", other.ID); err != nil {
		t.Fatal(err)
	}
	payment.ProviderOrderID = other.ID
	if _, err := f.store.approveVerifiedPayment(payment); err == nil {
		t.Fatal("the same provider transaction approved two orders")
	}
}
