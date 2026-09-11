// Command opnsense-smoke is a connectivity/auth smoke test for the OPNsense
// API: it reads the Dnsmasq DHCP leases and prints them, so we can confirm API
// access and field names before wiring the DNS-naming sync.
//
//	# fill OPNSENSE_* in .env, then:
//	go run ./cmd/opnsense-smoke
//	OPNSENSE_RAW=1 go run ./cmd/opnsense-smoke   # dump raw JSON
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/TimKue/omada2mqtt/internal/config"
	"github.com/TimKue/omada2mqtt/pkg/opnsense"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if cfg.OPNsenseURL == "" || cfg.OPNsenseKey == "" || cfg.OPNsenseSecret == "" {
		return fmt.Errorf("OPNSENSE_URL, OPNSENSE_KEY and OPNSENSE_SECRET must be set in .env")
	}

	opts := []opnsense.Option{}
	if cfg.OPNsenseInsecure {
		opts = append(opts, opnsense.WithInsecureTLS())
	}
	oc := opnsense.NewClient(cfg.OPNsenseURL, cfg.OPNsenseKey, cfg.OPNsenseSecret, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if os.Getenv("OPNSENSE_RAW") != "" {
		raw, err := oc.RawLeases(ctx)
		if err != nil {
			return fmt.Errorf("raw leases: %w", err)
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, raw, "", "  "); err != nil {
			return err
		}
		fmt.Printf("Dnsmasq leases — raw:\n%s\n", buf.String())
		return nil
	}

	leases, err := oc.Leases(ctx)
	if err != nil {
		return fmt.Errorf("leases: %w", err)
	}
	fmt.Printf("%d lease(s):\n", len(leases))
	named := 0
	for _, l := range leases {
		if l.Hostname != "" {
			named++
		}
		fmt.Printf("  %-17s %-15s %q\n", l.HWAddr, l.Address, l.Hostname)
	}
	fmt.Printf("\n%d of %d leases carry a hostname.\n", named, len(leases))
	return nil
}
