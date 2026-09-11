// Command omada-smoke is a connectivity/auth smoke test: it authenticates
// against the Omada Open API and prints the visible sites as JSON.
//
// Usage:
//
//	export OMADA_BASE_URL="https://192.168.178.2:8043"
//	export OMADA_ID="<omada id>"
//	export OMADA_CLIENT_ID="<client id>"
//	export OMADA_CLIENT_SECRET="<client secret>"
//	export OMADA_INSECURE=true   # local self-signed certificate
//	go run ./cmd/omada-smoke
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/TimKue/omada2mqtt/internal/config"
	"github.com/TimKue/omada2mqtt/pkg/omada"
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

	opts := []omada.Option{}
	if cfg.InsecureTLS {
		opts = append(opts, omada.WithInsecureTLS())
	}
	client := omada.NewClient(cfg.BaseURL, cfg.OmadacID, cfg.ClientID, cfg.ClientSecret, opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sites, err := client.ListSites(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("Authenticated. %d site(s).\n", len(sites))

	// Discovery helper: dump one device's raw detail (PoE/port fields).
	if mac := os.Getenv("OMADA_DETAIL_MAC"); mac != "" {
		if len(sites) == 0 {
			return fmt.Errorf("no sites available")
		}
		path, rd, err := client.RawDevice(ctx, sites[0].SiteID, mac)
		if err != nil {
			return fmt.Errorf("device detail %s: %w", mac, err)
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, rd, "", "  "); err != nil {
			return err
		}
		fmt.Printf("\nDevice %s — raw detail (via %s):\n%s\n", mac, path, buf.String())
		return nil
	}

	// Discovery helper: dump the site-wide switch PoE port info.
	if os.Getenv("OMADA_POE") != "" {
		if len(sites) == 0 {
			return fmt.Errorf("no sites available")
		}
		rd, err := client.RawSwitchPortsPoe(ctx, sites[0].SiteID)
		if err != nil {
			return fmt.Errorf("switch poe info: %w", err)
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, rd, "", "  "); err != nil {
			return err
		}
		fmt.Printf("\nSwitch ports PoE info:\n%s\n", buf.String())
		return nil
	}

	// Discovery helper: dump the site's clients (raw JSON or a table).
	if mode := os.Getenv("OMADA_CLIENTS"); mode != "" {
		if len(sites) == 0 {
			return fmt.Errorf("no sites available")
		}
		if mode == "raw" {
			rd, err := client.RawClients(ctx, sites[0].SiteID)
			if err != nil {
				return fmt.Errorf("raw clients: %w", err)
			}
			var buf bytes.Buffer
			if err := json.Indent(&buf, rd, "", "  "); err != nil {
				return err
			}
			fmt.Printf("\nClients — raw:\n%s\n", buf.String())
			return nil
		}
		clients, err := client.ListClients(ctx, sites[0].SiteID)
		if err != nil {
			return fmt.Errorf("list clients: %w", err)
		}
		fmt.Printf("\n%d client(s):\n", len(clients))
		for _, cl := range clients {
			conn := "wired"
			if cl.Wireless {
				conn = "wifi"
			}
			over := ""
			if cl.NameOverridden {
				over = " (name set)"
			}
			fmt.Printf("  %-17s %-15s %-6s name=%q host=%q%s\n",
				cl.Mac, cl.IP, conn, cl.Name, cl.HostName, over)
		}
		return nil
	}

	raw := os.Getenv("OMADA_RAW") != ""

	for _, s := range sites {
		if raw {
			rd, err := client.RawDevices(ctx, s.SiteID)
			if err != nil {
				return fmt.Errorf("raw devices for site %q: %w", s.Name, err)
			}
			var buf bytes.Buffer
			if err := json.Indent(&buf, rd, "", "  "); err != nil {
				return err
			}
			fmt.Printf("\nSite %q — raw device list:\n%s\n", s.Name, buf.String())
			continue
		}

		devices, err := client.ListDevices(ctx, s.SiteID)
		if err != nil {
			return fmt.Errorf("list devices for site %q: %w", s.Name, err)
		}
		fmt.Printf("\nSite %q — %d device(s):\n", s.Name, len(devices))
		for _, d := range devices {
			state := "offline"
			if d.Online() {
				state = "online"
			}
			fmt.Printf("  %-20s %-7s %-8s cpu %3d%%  mem %3d%%  %-15s up %s\n",
				d.Name, d.Type, state, d.CPUUtil, d.MemUtil, d.IP, d.Uptime)
		}

		ports, err := client.ListSwitchPortsPoe(ctx, s.SiteID)
		if err != nil {
			return fmt.Errorf("list switch PoE for site %q: %w", s.Name, err)
		}
		printPoE(ports)
	}
	return nil
}

// printPoE prints per-switch PoE totals and the currently active PoE ports.
func printPoE(ports []omada.SwitchPort) {
	totals := map[string]float64{}
	names := map[string]string{}
	var order []string
	for _, p := range ports {
		if _, seen := names[p.SwitchMac]; !seen {
			names[p.SwitchMac] = p.SwitchName
			order = append(order, p.SwitchMac)
		}
		totals[p.SwitchMac] += p.PoeWatts
	}
	if len(order) == 0 {
		return
	}
	fmt.Printf("\n  PoE per switch:\n")
	for _, mac := range order {
		fmt.Printf("    %-16s %5.1f W total\n", names[mac], totals[mac])
	}
	for _, p := range ports {
		if p.PoeWatts > 0 {
			fmt.Printf("      %s Port%d: %.1f W (%.1f V, %.0f mA)\n",
				p.SwitchName, p.Port, p.PoeWatts, p.PoeVolts, p.PoeMilliA)
		}
	}
}
