package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	helloVersion = "0.2"
)

var knownPaths []string
var boottime time.Time
var banner string
var requests int64
var usr *user.User

func inc() int64 {
	return atomic.AddInt64(&requests, 1)
}

func get() int64 {
	return atomic.LoadInt64(&requests)
}

func main() {

	boottime = time.Now()

	tls := true

	log.Print("version: ", helloVersion)
	log.Print("runtime: ", runtime.Version())
	log.Print("pid: ", os.Getpid())

	var errUser error
	usr, errUser = user.Current()
	if errUser != nil {
		log.Printf("current user error: %v", errUser)
	}
	if usr != nil {
		log.Printf("user: %s (uid: %s)", usr.Username, usr.Uid)
	}

	currDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Getwd: %s", err)
	}

	var addr, httpsAddr, key, cert string
	var disableKeepalive bool

	flag.StringVar(&key, "key", "key.pem", "TLS key file")
	flag.StringVar(&cert, "cert", "cert.pem", "TLS cert file")
	flag.StringVar(&addr, "addr", ":8080", "HTTP listen address")
	flag.StringVar(&httpsAddr, "httpsAddr", ":8443", "HTTPS listen address")
	flag.StringVar(&banner, "banner", "deploy #4", "banner will be displayed")
	flag.BoolVar(&disableKeepalive, "disableKeepalive", false, "disable keepalive")
	flag.Parse()

	keepalive := !disableKeepalive

	log.Print("banner: ", banner)
	log.Print("keepalive: ", keepalive)

	if !fileExists(key) {
		log.Printf("TLS key file not found: %s - disabling TLS", key)
		tls = false
	}

	if !fileExists(cert) {
		log.Printf("TLS cert file not found: %s - disabling TLS", cert)
		tls = false
	}

	// default handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { rootHandler(w, r, keepalive) })

	registerStatic("/www/", currDir)

	log.Printf("using TCP ports HTTP=%s HTTPS=%s TLS=%v", addr, httpsAddr, tls)

	if tls {
		serveHTTPS(addr, httpsAddr, cert, key, keepalive)
		return
	}

	log.Printf("serving HTTP on TCP %s", addr)
	if err := listenAndServe(addr, nil, keepalive); err != nil {
		log.Fatalf("listenAndServe: %s: %v", addr, err)
	}
}

func serveHTTPS(addr, httpsAddr, cert, key string, keepalive bool) {
	httpPort := "80"
	h := strings.Split(addr, ":")
	if len(h) > 1 {
		httpPort = h[1]
	}

	httpsPort := "443"
	hs := strings.Split(httpsAddr, ":")
	if len(hs) > 1 {
		httpsPort = hs[1]
	}

	if httpPort != httpsPort {
		log.Printf("installing redirect from HTTP=%s to HTTPS=%s", addr, httpsPort)

		redirectTLS := func(w http.ResponseWriter, r *http.Request) {
			host := strings.Split(r.Host, ":")[0]
			//code := http.StatusMovedPermanently // 301 permanent
			code := http.StatusFound // 302 temporary
			http.Redirect(w, r, "https://"+host+":"+httpsPort+r.RequestURI, code)
		}

		// http-to-https redirect server
		go func() {
			if err := listenAndServe(addr, http.HandlerFunc(redirectTLS), keepalive); err != nil {
				log.Fatalf("redirect: listenAndServe: %s: %v", addr, err)
			}
		}()
	}

	log.Printf("serving HTTPS on TCP %s", httpsAddr)
	if err := listenAndServeTLS(httpsAddr, cert, key, nil, keepalive); err != nil {
		log.Fatalf("listenAndServeTLS: %s: %v", httpsAddr, err)
	}
}

func listenAndServe(addr string, handler http.Handler, keepalive bool) error {
	server := &http.Server{Addr: addr, Handler: handler}
	server.SetKeepAlivesEnabled(keepalive)
	return server.ListenAndServe()
}

func listenAndServeTLS(addr, certFile, keyFile string, handler http.Handler, keepalive bool) error {
	server := &http.Server{Addr: addr, Handler: handler}
	server.SetKeepAlivesEnabled(keepalive)
	return server.ListenAndServeTLS(certFile, keyFile)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type staticHandler struct {
	innerHandler http.Handler
}

func registerStatic(path, dir string) {
	http.Handle(path, staticHandler{http.StripPrefix(path, http.FileServer(http.Dir(dir)))})
	knownPaths = append(knownPaths, path)
	log.Printf("mapping www path %s to directory %s", path, dir)
}

func (handler staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	count := inc()
	log.Printf("staticHandler.ServeHTTP req=%d url=%s from=%s", count, r.URL.Path, r.RemoteAddr)
	handler.innerHandler.ServeHTTP(w, r)
}

func rootHandler(w http.ResponseWriter, r *http.Request, keepalive bool) {
	count := inc()
	msg := fmt.Sprintf("rootHandler: req=%d url=%s from=%s", count, r.URL.Path, r.RemoteAddr)
	log.Print(msg)

	var paths string
	for _, p := range knownPaths {
		paths += fmt.Sprintf("<a href=\"%s\">%s</a> <br>", p, p)
	}

	var errMsg string
	if r.URL.Path != "/" {
		errMsg = fmt.Sprintf("<h2>Path not found!</h2>Path not found: [%s]", r.URL.Path)
	}

	header :=
		`<!DOCTYPE html>

<html>
  <head>
    <title>gowebhello root page</title>
  </head>
  <body>
    <h1>gowebhello root page</h1>
    <p>
    <a href="https://github.com/udhos/gowebhello">https://github.com/udhos/gowebhello</a> is a simple golang replacement for 'python -m SimpleHTTPServer'.
    </p>
`
	bodyTempl :=
		`<h2>Welcome!</h2>
	gowebhello version %s runtime %s<br>
	Application banner: %s<br>
	Application arguments: %v<br>
	Application dir: %s<br>
	Process: %d<br>
	User: %s (uid: %s)<br>
	Server hostname: %s<br>
	Your address: %s<br>
	Current time: %s<br>
	Uptime: %s<br>
	Requests: %d<br>
    %s
    <h2>All known paths:</h2>
    %s
`

	footer :=
		`</body>
</html>
`

	cwd, errCwd := os.Getwd()
	if errCwd != nil {
		cwd = cwd + " (error: " + errCwd.Error() + ")"
	}

	host, errHost := os.Hostname()
	if errHost != nil {
		host = host + " (error: " + errHost.Error() + ")"
	}

	now := time.Now()

	username := "?"
	uid := "?"

	if usr != nil {
		username = usr.Username
		uid = usr.Uid
	}

	body := fmt.Sprintf(bodyTempl, helloVersion, runtime.Version(), banner, os.Args, cwd, os.Getpid(), username, uid, host, r.RemoteAddr, now, time.Since(boottime), get(), errMsg, paths)

	if !keepalive {
		w.Header().Set("Connection", "close")
	}

	io.WriteString(w, header)
	io.WriteString(w, body)
	showHeaders(w, r)
	io.WriteString(w, footer)
}

func showHeaders(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "<h2>Headers</h2>\n")

	var headers []string
	for k, v := range r.Header {
		for _, vv := range v {
			headers = append(headers, k+": "+vv+"<br>\n")
		}
	}

	sort.Strings(headers)

	for _, h := range headers {
		io.WriteString(w, h)
	}
}
