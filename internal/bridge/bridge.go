// Package bridge orchestrates one poll cycle: it reads device state from an
// Omada controller and publishes Home Assistant discovery and state messages
// through a Publisher. It depends only on small interfaces, so it can be
// tested without a real controller or MQTT broker.
package bridge

import (
	"context"
	"fmt"

	"github.com/TimKue/omada2mqtt/internal/homeassistant"
	"github.com/TimKue/omada2mqtt/pkg/omada"
)

// Source is the subset of the Omada client the bridge needs.
type Source interface {
	ListSites(ctx context.Context) ([]omada.Site, error)
	ListDevices(ctx context.Context, siteID string) ([]omada.Device, error)
	ListSwitchPortsPoe(ctx context.Context, siteID string) ([]omada.SwitchPort, error)
}

// Publisher publishes one MQTT message. Implementations wrap a concrete MQTT
// client; the bridge stays free of any MQTT dependency.
type Publisher interface {
	Publish(topic string, payload []byte, retain bool) error
}

// Logf logs a non-fatal message (e.g. a single failed publish). It matches
// the signature of log.Printf / fmt.Printf-style loggers.
type Logf func(format string, args ...any)

// Bridge holds the state shared across poll cycles.
type Bridge struct {
	src       Source
	pub       Publisher
	ha        *homeassistant.Builder
	logf      Logf
	announced map[string]bool // device MAC -> discovery configs already sent
}

// New constructs a Bridge. logf may be nil, in which case non-fatal messages
// are dropped.
func New(src Source, pub Publisher, ha *homeassistant.Builder, logf Logf) *Bridge {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Bridge{
		src:       src,
		pub:       pub,
		ha:        ha,
		logf:      logf,
		announced: map[string]bool{},
	}
}

// Poll runs one full cycle over every site: it announces newly seen devices
// (once) and publishes current state for all of them. A publish failure for a
// single message is logged and does not abort the cycle; an error talking to
// the controller is returned.
func (b *Bridge) Poll(ctx context.Context) error {
	sites, err := b.src.ListSites(ctx)
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	for _, s := range sites {
		devices, err := b.src.ListDevices(ctx, s.SiteID)
		if err != nil {
			return fmt.Errorf("list devices for site %q: %w", s.Name, err)
		}

		// Per-switch PoE totals (watts), keyed by switch MAC. A failure here
		// is non-fatal: state is still published, just without PoE.
		poe := map[string]float64{}
		if ports, err := b.src.ListSwitchPortsPoe(ctx, s.SiteID); err != nil {
			b.logf("poe info for site %q: %v", s.Name, err)
		} else {
			for _, p := range ports {
				poe[p.SwitchMac] += p.PoeWatts
			}
		}

		for _, d := range devices {
			if !b.announced[d.Mac] {
				if b.announce(d) {
					b.announced[d.Mac] = true
				}
			}
			b.publishState(d, poe[d.Mac])
		}
	}
	return nil
}

// announce publishes the discovery configs for one device. It returns true
// once all configs were sent, so a device is not marked announced if MQTT was
// unavailable and can be retried on the next cycle.
func (b *Bridge) announce(d omada.Device) bool {
	msgs, err := b.ha.Discovery(d)
	if err != nil {
		b.logf("build discovery for %s: %v", d.Mac, err)
		return false
	}
	ok := true
	for _, m := range msgs {
		if err := b.pub.Publish(m.Topic, m.Payload, m.Retain); err != nil {
			b.logf("publish %s: %v", m.Topic, err)
			ok = false
		}
	}
	return ok
}

func (b *Bridge) publishState(d omada.Device, poeWatts float64) {
	m, err := b.ha.State(d, poeWatts)
	if err != nil {
		b.logf("build state for %s: %v", d.Mac, err)
		return
	}
	if err := b.pub.Publish(m.Topic, m.Payload, m.Retain); err != nil {
		b.logf("publish %s: %v", m.Topic, err)
	}
}
