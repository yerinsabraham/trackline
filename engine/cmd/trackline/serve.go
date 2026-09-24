package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/yerinsabraham/trackline/engine/internal/config"
	"github.com/yerinsabraham/trackline/engine/internal/production"
)

// cmdServe receives traces from a running agent.
//
//	trackline serve [--addr 127.0.0.1:4318] [--root DIR] [--sample 1] [--alert URL] [--out FILE]
//
// Point an OTLP/HTTP exporter at http://ADDR/v1/traces. Localhost by default:
// traces carry customer messages, and a receiver open to the network is a
// decision for whoever runs it, not a default.
func cmdServe(args []string) error {
	addr := "127.0.0.1:4318"
	root, _ := os.Getwd()
	sample := 1.0
	alert, out := "", ""
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			break
		}
		switch args[i] {
		case "--addr":
			i++
			addr = args[i]
		case "--root", "-root":
			i++
			root = args[i]
		case "--sample":
			i++
			v, err := strconv.ParseFloat(args[i], 64)
			if err != nil || v <= 0 || v > 1 {
				return fmt.Errorf("--sample must be between 0 and 1, got %q", args[i])
			}
			sample = v
		case "--alert":
			i++
			alert = args[i]
		case "--out":
			i++
			out = args[i]
		}
	}

	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	srv := production.NewServer(cfg)
	srv.Sample = sample

	var mu sync.Mutex
	var log *json.Encoder
	if out != "" {
		f, err := os.OpenFile(out, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		log = json.NewEncoder(f)
	}
	checked := 0
	srv.Emit = func(r production.Result) {
		mu.Lock()
		defer mu.Unlock()
		checked++
		if log != nil {
			log.Encode(r)
		}
		if len(r.Findings()) > 0 || len(r.Incidents) > 0 {
			fmt.Println(production.Summary(r))
		}
	}
	if alert != "" {
		srv.Alert = production.Webhook(alert)
	}

	fmt.Printf("trackline: receiving traces on http://%s/v1/traces\n", addr)
	if cfg.Tools.Empty() {
		fmt.Println("no tool policy in .trackline.json; watching for outages, loops and drift only")
	}
	if sample < 1 {
		fmt.Printf("checking %.0f%% of conversations, chosen whole\n", sample*100)
	}

	go func() {
		for range time.Tick(5 * time.Second) {
			srv.Flush(false)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	errc := make(chan error, 1)
	go func() { errc <- http.ListenAndServe(addr, srv) }()

	select {
	case err := <-errc:
		return err
	case <-stop:
		// Whatever is still waiting for its root is checked, not dropped.
		srv.Flush(true)
		mu.Lock()
		fmt.Printf("\nchecked %d conversations\n", checked)
		mu.Unlock()
		return nil
	}
}
