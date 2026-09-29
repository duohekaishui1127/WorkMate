package main

import (
 "crypto/ed25519"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "flag"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "time"
)

type payload struct{Version int `json:"v"`; LicenseID string `json:"license_id"`; Edition string `json:"edition"`; DeviceID string `json:"device_id"`; IssuedAt string `json:"issued_at"`; Customer string `json:"customer,omitempty"`}

func main(){
 gen:=flag.Bool("generate",false,"generate a fresh Ed25519 release key pair and inject the public key into licensing.go")
 key:=flag.String("key","developer-secrets/license-private.key","base64 Ed25519 private seed file")
 dev:=flag.String("device","","full 64-char device hash")
 id:=flag.String("id","","license id, e.g. WM-20260929-0001")
 customer:=flag.String("customer","","optional customer label")
 flag.Parse()
 if *gen { generateKeys(); return }
 if *dev==""||*id==""{fmt.Fprintln(os.Stderr,"usage: go run ./tools/licensegen -device <hash> -id <license-id> [-customer name]");os.Exit(2)}
 kb,err:=os.ReadFile(*key);if err!=nil{panic(err)};raw,err:=base64.StdEncoding.DecodeString(strings.TrimSpace(string(kb)));if err!=nil||len(raw)!=ed25519.SeedSize{panic("private key must be 32-byte Ed25519 seed in base64")}
 priv:=ed25519.NewKeyFromSeed(raw); p:=payload{1,*id,"pro",strings.ToLower(strings.TrimSpace(*dev)),time.Now().UTC().Format(time.RFC3339),*customer}; b,_:=json.Marshal(p); sig:=ed25519.Sign(priv,b)
 fmt.Println(base64.RawURLEncoding.EncodeToString(b)+"."+base64.RawURLEncoding.EncodeToString(sig))
}

func generateKeys(){
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{panic(err)}
 if err:=os.MkdirAll("developer-secrets",0700);err!=nil{panic(err)}
 seed:=priv.Seed(); privB64:=base64.StdEncoding.EncodeToString(seed); pubB64:=base64.StdEncoding.EncodeToString(pub)
 if err:=os.WriteFile(filepath.Join("developer-secrets","license-private.key"),[]byte(privB64+"\n"),0600);err!=nil{panic(err)}
 if err:=os.WriteFile(filepath.Join("developer-secrets","license-public.key"),[]byte(pubB64+"\n"),0644);err!=nil{panic(err)}
 p:="licensing.go"; src,err:=os.ReadFile(p);if err!=nil{panic(err)}; text:=string(src)
 marker:="const releasePublicKeyB64 = \""; a:=strings.Index(text,marker);if a<0{panic("releasePublicKeyB64 not found")}; a+=len(marker); b:=strings.Index(text[a:],"\"");if b<0{panic("public key constant malformed")}; b+=a
 text=text[:a]+pubB64+text[b:]; if err:=os.WriteFile(p,[]byte(text),0644);err!=nil{panic(err)}
 fmt.Println("New release keys generated.")
 fmt.Println("PRIVATE KEY:",filepath.Join("developer-secrets","license-private.key"),"(NEVER distribute this file)")
 fmt.Println("Public key injected into licensing.go.")
}
