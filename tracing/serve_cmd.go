package tracing

import (
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/liangdas/mqant/log"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"strings"

	"sourcegraph.com/sourcegraph/appdash"
	"sourcegraph.com/sourcegraph/appdash/traceapp"
)

// ServeCmd is the command for running Appdash in server mode, where a
// collector server and the web UI are hosted.
type ServeCmd struct {
	URL           string `long:"url" description:"URL which Appdash is being hosted at (e.g. http://localhost:7700)"`
	CollectorAddr string `long:"collector" description:"collector listen address" default:":7701"`
	HTTPAddr      string `long:"http" description:"HTTP listen address" default:":7700"`

	StoreFile       string        `short:"f" long:"store-file" description:"persisted store file" default:"/tmp/appdash.gob"`
	PersistInterval time.Duration `short:"p" long:"persist-interval" description:"interval between persisting store to file" default:"2s"`

	Debug bool `short:"d" long:"debug" description:"debug log"`
	Trace bool `long:"trace" description:"trace log"`

	DeleteAfter time.Duration `long:"delete-after" description:"delete traces after a certain age (0 to disable)" default:"24h"`

	LimitMax int `short:"m" long:"limit-max" description:"Max is the maximum number of traces that the store should keep." default:"100000"`

	TLSCert string `long:"tls-cert" description:"TLS certificate file (if set, enables TLS)"`
	TLSKey  string `long:"tls-key" description:"TLS key file (if set, enables TLS)"`

	BasicAuth string `long:"basic-auth" description:"if set to 'user:passwd', require HTTP Basic Auth for web app"`
}

// Execute execudes the commands with the given arguments and returns an error,
// if any.
func (c *ServeCmd) Execute(httplisten net.Listener) error {
	var (
		memStore = appdash.NewMemoryStore()
		Store    = appdash.Store(memStore)
		Queryer  = memStore
	)

	if c.StoreFile != "" {
		f, err := os.Open(c.StoreFile)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if f != nil {
			if n, err := memStore.ReadFrom(f); err == nil {
				log.Info("Read %d traces from file %s", n, c.StoreFile)
			} else if err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
		if c.PersistInterval != 0 {
			go func() {
				if err := appdash.PersistEvery(memStore, c.PersistInterval, c.StoreFile); err != nil {
					log.Error("appdash.PersistEvery", err.Error())
				}
			}()
		}
	}

	if c.DeleteAfter > 0 {
		Store = &appdash.RecentStore{
			MinEvictAge: c.DeleteAfter,
			DeleteStore: memStore,
			Debug:       true,
		}
	}

	if c.LimitMax > 0 {
		Store = &appdash.LimitStore{
			Max:         c.LimitMax,
			DeleteStore: memStore,
		}
	}

	url, err := c.urlOrDefault()
	if err != nil {
		return err
	}
	app, err := traceapp.New(nil, url)
	if err != nil {
		return err
	}
	app.Store = Store
	app.Queryer = Queryer

	var h http.Handler
	if c.BasicAuth != "" {
		parts := strings.SplitN(c.BasicAuth, ":", 2)
		if len(parts) != 2 {
			log.Error("Basic auth must be specified as 'user:passwd'.")
		}
		user, passwd := parts[0], parts[1]
		if user == "" || passwd == "" {
			log.Error("Basic auth user and passwd must both be nonempty.")
		}
		log.Info("Requiring HTTP Basic auth")
		h = newBasicAuthHandler(user, passwd, app)
	} else {
		h = app
	}

	var l net.Listener
	var proto string
	if c.TLSCert != "" || c.TLSKey != "" {
		certBytes, err := ioutil.ReadFile(c.TLSCert)
		if err != nil {
			return err
		}
		keyBytes, err := ioutil.ReadFile(c.TLSKey)
		if err != nil {
			return err
		}

		var tc tls.Config
		cert, err := tls.X509KeyPair(certBytes, keyBytes)
		if err != nil {
			return err
		}
		tc.Certificates = []tls.Certificate{cert}
		l, err = tls.Listen("tcp", c.CollectorAddr, &tc)
		if err != nil {
			return err
		}
		proto = fmt.Sprintf("TLS cert %s, key %s", c.TLSCert, c.TLSKey)
	} else {
		var err error
		l, err = net.Listen("tcp", c.CollectorAddr)
		if err != nil {
			return err
		}
		proto = "plaintext TCP (no security)"
	}
	log.Info("appdash collector listening on %s (%s)", c.CollectorAddr, proto)
	cs := appdash.NewServer(l, appdash.NewLocalCollector(Store))
	cs.Debug = c.Debug
	cs.Trace = c.Trace
	go cs.Start()

	if c.TLSCert != "" || c.TLSKey != "" {
		log.Info("appdash HTTPS server listening on %s (TLS cert %s, key %s)", c.HTTPAddr, c.TLSCert, c.TLSKey)
		tlsConf := new(tls.Config)
		tlsConf.Certificates = make([]tls.Certificate, 1)
		tlsConf.Certificates[0], err = tls.LoadX509KeyPair(c.TLSCert, c.TLSKey)
		if err == nil {
			httplisten = tls.NewListener(httplisten, tlsConf)
			log.Info("TCP Listen TLS load success")
		} else {
			log.Warning("tcp_server tls :%v", err)
		}
	}

	log.Info("appdash HTTP server listening on %s", c.HTTPAddr)
	return http.Serve(httplisten, h)
}

// urlOrDefault returns c.URL if non-empty, otherwise it returns c.HTTPAddr
// with localhost" as the default host (if not specified in c.HTTPAddr).
func (c *ServeCmd) urlOrDefault() (*url.URL, error) {
	// Parse c.URL and return it if non-empty.
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, err
	}
	if c.URL != "" {
		return u, nil
	}

	// Parse c.HTTPAddr and use a default host if not specified.
	host, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "localhost"
	}
	addr := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	return addr, nil
}

func newBasicAuthHandler(user, passwd string, h http.Handler) http.Handler {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", user, passwd)))
	return &basicAuthHandler{h, []byte(want)}
}

type basicAuthHandler struct {
	http.Handler
	want []byte // = "Basic " base64(user ":" passwd) [precomputed]
}

func (h *basicAuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Constant time comparison to avoid timing attack.
	authHdr := r.Header.Get("authorization")
	if len(h.want) == len(authHdr) && subtle.ConstantTimeCompare(h.want, []byte(authHdr)) == 1 {
		h.Handler.ServeHTTP(w, r)
		return
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="appdash"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
