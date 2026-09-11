// Command omada-dns-sync names Omada clients from OPNsense as the single source
// of truth: it reads the Dnsmasq DHCP leases (MAC -> hostname) and writes those
// hostnames as the clients' display names in the Omada controller.
//
// It is safe by default: without -apply it only prints what it *would* change
// (dry run). Use -apply to write, and -limit to cap the number of writes for a
// cautious first run.
//
//	go run ./cmd/omada-dns-sync                 # dry run: show planned changes
//	go run ./cmd/omada-dns-sync -apply -limit 1 # write just the first change
//	go run ./cmd/omada-dns-sync -apply          # write all changes
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/TimKue/omada2mqtt/internal/config"
	"github.com/TimKue/omada2mqtt/internal/dnssync"
	"github.com/TimKue/omada2mqtt/pkg/omada"
	"github.com/TimKue/omada2mqtt/pkg/opnsense"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	apply := flag.Bool("apply", false, "actually write names (default: dry run)")
	limit := flag.Int("limit", 0, "max number of writes (0 = no limit)")
	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if cfg.OPNsenseURL == "" || cfg.OPNsenseKey == "" || cfg.OPNsenseSecret == "" {
		return fmt.Errorf("OPNSENSE_URL, OPNSENSE_KEY and OPNSENSE_SECRET must be set in .env")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// OPNsense: build the desired MAC -> hostname map (the source of truth).
	var oo []opnsense.Option
	if cfg.OPNsenseInsecure {
		oo = append(oo, opnsense.WithInsecureTLS())
	}
	opn := opnsense.NewClient(cfg.OPNsenseURL, cfg.OPNsenseKey, cfg.OPNsenseSecret, oo...)
	leases, err := opn.Leases(ctx)
	if err != nil {
		return fmt.Errorf("read OPNsense leases: %w", err)
	}
	want := dnssync.DesiredNames(leases)
	fmt.Printf("OPNsense: %d leases, %d with a hostname\n", len(leases), len(want))

	// Omada: list clients per site and compute the plan.
	var od []omada.Option
	if cfg.InsecureTLS {
		od = append(od, omada.WithInsecureTLS())
	}
	oc := omada.NewClient(cfg.BaseURL, cfg.OmadacID, cfg.ClientID, cfg.ClientSecret, od...)

	sites, err := oc.ListSites(ctx)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}

	var plan []dnssync.Change
	for _, s := range sites {
		clients, err := oc.ListClients(ctx, s.SiteID)
		if err != nil {
			return fmt.Errorf("list clients for site %q: %w", s.Name, err)
		}
		plan = append(plan, dnssync.Plan(s.SiteID, clients, want)...)
	}

	if len(plan) == 0 {
		fmt.Println("Nothing to do — every matched client already has its OPNsense name.")
		return nil
	}

	fmt.Printf("\n%d client(s) would be renamed:\n", len(plan))
	for _, c := range plan {
		fmt.Printf("  %-17s %-28q -> %q\n", c.Mac, c.Old, c.New)
	}

	if !*apply {
		fmt.Println("\nDry run — nothing written. Re-run with -apply to write (add -limit N to cap).")
		return nil
	}

	n := 0
	for _, c := range plan {
		if *limit > 0 && n >= *limit {
			fmt.Printf("\nReached -limit %d, stopping.\n", *limit)
			break
		}
		if err := oc.SetClientName(ctx, c.SiteID, c.Mac, c.New); err != nil {
			// Fail fast: a write error is almost always systemic (permissions,
			// auth, wrong endpoint), so stop rather than spamming every client.
			fmt.Fprintf(os.Stderr, "  FAILED %s -> %q: %v\n", c.Mac, c.New, err)
			fmt.Fprintln(os.Stderr, "stopping after first error (no further writes attempted).")
			return fmt.Errorf("write failed after %d successful name(s)", n)
		}
		fmt.Printf("  set %s -> %q\n", c.Mac, c.New)
		n++
	}
	fmt.Printf("\nDone: %d name(s) written.\n", n)
	return nil
}
