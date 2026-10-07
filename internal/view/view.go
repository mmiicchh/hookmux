// Package view serves the history as a small local web page.
package view

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/mmiicchh/hookmux/internal/hist"
)

//go:embed index.html
var index []byte

type Options struct {
	Hist        hist.Store
	Port        int
	OpenBrowser bool
	Log         io.Writer
}

func Serve(o Options) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index)
	})
	mux.HandleFunc("GET /api/hist", func(w http.ResponseWriter, r *http.Request) {
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = hist.DefaultLimit
		}
		recs, err := o.Hist.List(limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recs)
	})
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", o.Port))
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()
	fmt.Fprintln(o.Log, "hookmux view:", url)
	if o.OpenBrowser {
		openBrowser(url)
	}
	return http.Serve(ln, mux)
}

func openBrowser(url string) {
	bin := "xdg-open"
	if runtime.GOOS == "darwin" {
		bin = "open"
	}
	exec.Command(bin, url).Start()
}
