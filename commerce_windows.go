//go:build windows

package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"time"
	"unsafe"
)

type purchaseResult struct {
	kind   string
	cfg    *onlineConfig
	ticket *purchaseTicket
	order  *onlineOrder
	err    error
	manual bool
}
type onlinePurchaseUI struct {
	client               *purchaseClient
	results              chan purchaseResult
	config               *onlineConfig
	lastOrder            *onlineOrder
	busy                 bool
	nextSync, nextConfig time.Time
	status               string
	setupError           error
}

var onlinePurchase *onlinePurchaseUI

func initOnlinePurchase() {
	cfg, err := loadCommerceConfig(exeDir())
	if err != nil || cfg.ServerURL == "" {
		return
	}
	onlinePurchase = &onlinePurchaseUI{results: make(chan purchaseResult, 2), status: "正在连接购买后台…"}
	key, e := base64.StdEncoding.DecodeString(onlineLicensePublicKeyB64)
	if e != nil || len(key) != ed25519.PublicKeySize {
		onlinePurchase.setupError = fmt.Errorf("此构建尚未配置后台授权公钥，请由开发者重新构建")
		return
	}
	onlinePurchase.client, onlinePurchase.setupError = newPurchaseClient(cfg.ServerURL, app.license.deviceID, app.store.DataDir)
}
func syncOnlinePurchase(now time.Time, manual bool) {
	u := onlinePurchase
	if u == nil || u.client == nil || u.busy || (!manual && now.Before(u.nextSync)) {
		return
	}
	u.busy = true
	u.nextSync = now.Add(30 * time.Second)
	client := u.client
	var ticket *purchaseTicket
	if client.ticket != nil {
		copy := *client.ticket
		ticket = &copy
	}
	wantConfig := u.config == nil || manual || !now.Before(u.nextConfig)
	if wantConfig {
		u.nextConfig = now.Add(5 * time.Minute)
	}
	results := u.results
	go func() {
		r := purchaseResult{kind: "sync", manual: manual}
		if wantConfig {
			cfg, err := client.config()
			if err == nil {
				r.cfg = &cfg
			} else {
				r.err = err
			}
		}
		if ticket != nil {
			order, err := client.status(*ticket)
			if err == nil {
				r.order = &order
			} else {
				r.err = err
			}
		}
		results <- r
	}()
}
func beginOnlinePurchase() {
	u := onlinePurchase
	if u == nil {
		return
	}
	if u.setupError != nil {
		msgBox(purchaseWnd, "购买后台暂不可用", u.setupError.Error(), MB_OK|MB_ICONWARNING)
		return
	}
	if u.client.ticket != nil {
		openOnlinePurchasePage()
		syncOnlinePurchase(time.Now(), true)
		return
	}
	if app.license.IsPro() {
		msgBox(purchaseWnd, "已开通 Pro", "当前电脑已永久解锁，无需重复购买。", MB_OK|MB_ICONINFORMATION)
		return
	}
	if u.busy {
		u.status = "正在连接后台，请稍候再试。"
		refreshOnlinePurchaseWindow()
		return
	}
	u.busy = true
	u.status = "正在创建购买订单…"
	refreshOnlinePurchaseWindow()
	client, results := u.client, u.results
	go func() {
		t, order, err := client.create()
		r := purchaseResult{kind: "create", err: err, manual: true}
		if err == nil {
			r.ticket = &t
			r.order = &order
		}
		results <- r
	}()
}
func openOnlinePurchasePage() {
	if onlinePurchase == nil || onlinePurchase.client == nil {
		return
	}
	target := onlinePurchase.client.purchaseURL()
	if target == "" {
		return
	}
	r, _, _ := pShellExecute.Call(uintptr(purchaseWnd), 0, uintptr(unsafe.Pointer(u16(target))), 0, 0, SW_SHOWNORMAL)
	if int(r) <= 32 {
		msgBox(purchaseWnd, "无法打开购买页", "请设置默认浏览器，然后重新打开订单。", MB_OK|MB_ICONWARNING)
	}
}
func drainOnlinePurchase() {
	u := onlinePurchase
	if u == nil {
		return
	}
	for {
		select {
		case r := <-u.results:
			u.busy = false
			if r.cfg != nil {
				if err := app.license.ApplyTrialPolicy(r.cfg.TrialPolicy); err != nil {
					r.err = fmt.Errorf("后台试用策略签名校验失败，请联系开发者")
				} else {
					u.config = r.cfg
				}
			}
			if r.ticket != nil {
				if err := u.client.saveTicket(*r.ticket); err != nil {
					r.err = fmt.Errorf("订单已创建，但本机保存失败，请保持软件运行并检查磁盘权限")
				}
			}
			if r.order != nil {
				u.lastOrder = r.order
				u.status = onlineOrderText(*r.order)
				if r.order.Status == "approved" && !app.license.IsPro() {
					if err := app.license.Activate(r.order.LicenseCode); err != nil {
						r.err = err
					} else {
						u.status = "Pro 已永久解锁，授权已保存到本机。"
						showBalloon("WorkMate Pro 已开通", "付款已确认，当前电脑已永久解锁。", false)
						invalidate(app.main)
					}
				}
			} else if r.cfg != nil {
				if r.cfg.Enabled && r.cfg.QRAvailable {
					u.status = "可创建订单，付款后等待开发者人工核实。"
				} else {
					u.status = "购买入口尚未开放。已有付款订单仍可检查。"
				}
			}
			if r.err != nil {
				u.status = r.err.Error()
				if r.manual && purchaseWnd != 0 {
					msgBox(purchaseWnd, "订单暂未完成", r.err.Error(), MB_OK|MB_ICONWARNING)
				}
			}
			refreshOnlinePurchaseWindow()
			if r.kind == "create" && r.ticket != nil && u.client.ticket != nil {
				openOnlinePurchasePage()
			}
		default:
			return
		}
	}
}
func onlineOrderText(o onlineOrder) string {
	switch o.Status {
	case "created":
		return "订单 " + o.ID + "：请在购买页提交付款信息。"
	case "pending":
		return "订单 " + o.ID + "：待开发者人工核实，请勿重复付款。"
	case "rejected":
		return "订单需补充信息：" + o.AdminNote
	case "approved":
		return "Pro 已开通，授权已保存到本机。"
	}
	return ""
}
func refreshOnlinePurchaseWindow() {
	u := onlinePurchase
	if purchaseWnd == 0 || u == nil {
		return
	}
	if u.setupError != nil {
		setText(getDlgItem(purchaseWnd, 511), u.setupError.Error())
		return
	}
	setText(getDlgItem(purchaseWnd, 511), u.status)
	if u.config != nil {
		if u.config.Enabled && u.config.QRAvailable {
			setText(getDlgItem(purchaseWnd, 508), fmt.Sprintf("买断价格：¥%.2f", float64(u.config.PriceCents)/100))
		} else {
			setText(getDlgItem(purchaseWnd, 508), "购买入口尚未开放。已有订单仍可处理。")
		}
		setText(getDlgItem(purchaseWnd, 509), "联系开发者："+u.config.Contact)
	}
	if u.client.ticket != nil {
		setText(getDlgItem(purchaseWnd, 501), "① 打开已有付款订单")
	}
	enabled := uintptr(1)
	if u.busy {
		enabled = 0
	}
	pEnableWindow.Call(uintptr(getDlgItem(purchaseWnd, 501)), enabled)
	pEnableWindow.Call(uintptr(getDlgItem(purchaseWnd, 510)), enabled)
}
