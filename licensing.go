package main

import (
    "crypto/ed25519"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"
)

const trialDuration = 24 * time.Hour
const trialPepper = "WorkMate-V8-Trial-Anchor-2026-09"

// IMPORTANT: this public key is safe to ship. The private key must never be
// included in the user installer. Regenerate the release key before selling.
const releasePublicKeyB64 = "7mdsvGNmNACC+mcYCCYGale1s0V19Ea5sv2sXfgWqlg="

type licensePayload struct {
    Version   int    `json:"v"`
    LicenseID string `json:"license_id"`
    Edition   string `json:"edition"`
    DeviceID  string `json:"device_id"`
    IssuedAt  string `json:"issued_at"`
    Customer  string `json:"customer,omitempty"`
}

type trialState struct {
    DeviceID   string `json:"device_id"`
    StartedAt  string `json:"started_at"`
    LastSeenAt string `json:"last_seen_at"`
}

type LicenseManager struct {
    dataDir string
    deviceID string
    pro bool
    trialStart time.Time
    trialLastSeen time.Time
    clockRollback bool
    lastPersist time.Time
}

func newLicenseManager(dataDir string) *LicenseManager {
    lm := &LicenseManager{dataDir:dataDir, deviceID: deviceFingerprint()}
    lm.loadLicense()
    lm.loadOrCreateTrial()
    return lm
}

func (lm *LicenseManager) DeviceCode() string { return strings.ToUpper(lm.deviceID[:minInt(20,len(lm.deviceID))]) }
func (lm *LicenseManager) IsPro() bool { return lm.pro }
func (lm *LicenseManager) TrialActive(now time.Time) bool { return !lm.pro && !lm.clockRollback && now.Before(lm.trialStart.Add(trialDuration)) }
func (lm *LicenseManager) HasProAccess(now time.Time) bool { return lm.pro || lm.TrialActive(now) }
func (lm *LicenseManager) TrialRemaining(now time.Time) time.Duration {
    if lm.pro { return 0 }
    d := lm.trialStart.Add(trialDuration).Sub(now); if d < 0 { return 0 }; return d
}
func (lm *LicenseManager) StatusText(now time.Time) string {
    if lm.pro { return "PRO · 永久版" }
    if lm.TrialActive(now) {
        d:=lm.TrialRemaining(now); h:=int(d.Hours()); m:=int(d.Minutes())%60
        return fmt.Sprintf("PRO 试用 · %dh%02dm", h,m)
    }
    return "FREE · 试用已结束"
}
func (lm *LicenseManager) Tick(now time.Time) {
    if lm.pro { return }
    if !lm.trialLastSeen.IsZero() && now.Before(lm.trialLastSeen.Add(-10*time.Minute)) { lm.clockRollback = true }
    if now.After(lm.trialLastSeen) { lm.trialLastSeen = now }
    if time.Since(lm.lastPersist) > time.Minute { lm.persistTrial(); lm.lastPersist=time.Now() }
}

func (lm *LicenseManager) trialPath() string { return filepath.Join(lm.dataDir,"commerce-state.dat") }
func sealTrial(st trialState) string {
    b,_:=json.Marshal(st); enc:=base64.RawURLEncoding.EncodeToString(b)
    mac:=hmac.New(sha256.New,[]byte(trialPepper)); mac.Write([]byte(enc)); return enc+"."+hex.EncodeToString(mac.Sum(nil))
}
func unsealTrial(s string) (trialState,error) {
    var st trialState; parts:=strings.Split(strings.TrimSpace(s),"."); if len(parts)!=2 { return st,errors.New("bad trial token") }
    mac:=hmac.New(sha256.New,[]byte(trialPepper)); mac.Write([]byte(parts[0])); want:=mac.Sum(nil); got,err:=hex.DecodeString(parts[1]); if err!=nil || !hmac.Equal(want,got) { return st,errors.New("trial token modified") }
    b,err:=base64.RawURLEncoding.DecodeString(parts[0]); if err!=nil { return st,err }; err=json.Unmarshal(b,&st); return st,err
}
func (lm *LicenseManager) loadOrCreateTrial() {
    var states []trialState
    if b,err:=os.ReadFile(lm.trialPath()); err==nil { if st,e:=unsealTrial(string(b)); e==nil && st.DeviceID==lm.deviceID { states=append(states,st) } }
    if s,err:=readSecurityRegistry("TrialState"); err==nil && s!="" { if st,e:=unsealTrial(s); e==nil && st.DeviceID==lm.deviceID { states=append(states,st) } }
    now:=time.Now().UTC()
    if len(states)==0 { lm.trialStart=now; lm.trialLastSeen=now; lm.persistTrial(); return }
    start:=now; last:=time.Time{}
    for _,st:=range states { a,_:=time.Parse(time.RFC3339,st.StartedAt); l,_:=time.Parse(time.RFC3339,st.LastSeenAt); if !a.IsZero() && a.Before(start) { start=a }; if l.After(last) { last=l } }
    lm.trialStart=start; lm.trialLastSeen=last
    if now.Before(last.Add(-10*time.Minute)) { lm.clockRollback=true }
    lm.persistTrial() // heal one missing/edited anchor from the other
}
func (lm *LicenseManager) persistTrial() {
    st:=trialState{DeviceID:lm.deviceID,StartedAt:lm.trialStart.UTC().Format(time.RFC3339),LastSeenAt:lm.trialLastSeen.UTC().Format(time.RFC3339)}
    tok:=sealTrial(st); _=os.WriteFile(lm.trialPath(),[]byte(tok),0600); _=writeSecurityRegistry("TrialState",tok)
}

func (lm *LicenseManager) licensePath() string { return filepath.Join(lm.dataDir,"license.dat") }
func (lm *LicenseManager) loadLicense() { if b,err:=os.ReadFile(lm.licensePath()); err==nil { if lm.verifyActivationCode(strings.TrimSpace(string(b)))==nil { lm.pro=true } } }
func (lm *LicenseManager) Activate(code string) error {
    if err:=lm.verifyActivationCode(strings.TrimSpace(code)); err!=nil { return err }
    if err:=os.WriteFile(lm.licensePath(),[]byte(strings.TrimSpace(code)),0600); err!=nil { return err }; lm.pro=true; return nil
}
func (lm *LicenseManager) verifyActivationCode(code string) error {
    parts:=strings.Split(code,"."); if len(parts)!=2 { return errors.New("激活码格式不正确") }
    pubBytes,err:=base64.StdEncoding.DecodeString(releasePublicKeyB64); if err!=nil || len(pubBytes)!=ed25519.PublicKeySize { return errors.New("此测试构建尚未配置正式授权公钥") }
    payload,err:=base64.RawURLEncoding.DecodeString(parts[0]); if err!=nil { return errors.New("激活码内容损坏") }
    sig,err:=base64.RawURLEncoding.DecodeString(parts[1]); if err!=nil || !ed25519.Verify(ed25519.PublicKey(pubBytes),payload,sig) { return errors.New("激活码签名无效") }
    var p licensePayload; if json.Unmarshal(payload,&p)!=nil { return errors.New("激活码内容无效") }
    if p.Edition!="pro" { return errors.New("不是 Pro 授权") }
    if p.DeviceID!=lm.deviceID { return errors.New("此激活码不属于当前电脑") }
    return nil
}
func deviceFingerprint() string {
    raw,_:=readMachineGuid(); if strings.TrimSpace(raw)=="" { h,_:=os.Hostname(); raw=h+"|"+os.Getenv("USERNAME") }
    sum:=sha256.Sum256([]byte("WorkMate|"+strings.TrimSpace(raw))); return hex.EncodeToString(sum[:])
}
