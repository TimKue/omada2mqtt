package bridge

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/TimKue/omada2mqtt/internal/homeassistant"
	"github.com/TimKue/omada2mqtt/pkg/omada"
)

// fakeSource is a test double for the Omada controller.
type fakeSource struct {
	sites   []omada.Site
	devices map[string][]omada.Device
	ports   map[string][]omada.SwitchPort
	err     error
}

func (f *fakeSource) ListSites(context.Context) ([]omada.Site, error) {
	return f.sites, f.err
}

func (f *fakeSource) ListDevices(_ context.Context, siteID string) ([]omada.Device, error) {
	return f.devices[siteID], nil
}

func (f *fakeSource) ListSwitchPortsPoe(_ context.Context, siteID string) ([]omada.SwitchPort, error) {
	return f.ports[siteID], nil
}

// recordingPublisher records every published message.
type recordingPublisher struct {
	mu   sync.Mutex
	msgs []string // topics
	fail bool
}

func (p *recordingPublisher) Publish(topic string, _ []byte, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fail {
		return errors.New("broker down")
	}
	p.msgs = append(p.msgs, topic)
	return nil
}

func (p *recordingPublisher) topics() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.msgs...)
}

func newTestBridge(src Source, pub Publisher) *Bridge {
	ha := homeassistant.New("omada2mqtt", "omada2mqtt/bridge/availability", "")
	return New(src, pub, ha, nil)
}

func countPrefix(topics []string, prefix string) int {
	n := 0
	for _, t := range topics {
		if strings.HasPrefix(t, prefix) {
			n++
		}
	}
	return n
}

func TestPollAnnouncesOnceThenStateOnly(t *testing.T) {
	src := &fakeSource{
		sites: []omada.Site{{SiteID: "s1", Name: "home"}},
		devices: map[string][]omada.Device{
			"s1": {
				{Mac: "AA-00-00-00-00-01", Name: "Core", Type: "switch"},
				{Mac: "AA-00-00-00-00-02", Name: "AP", Type: "ap"},
			},
		},
		ports: map[string][]omada.SwitchPort{
			"s1": {{SwitchMac: "AA-00-00-00-00-01", Port: 1, PoeWatts: 5}},
		},
	}
	pub := &recordingPublisher{}
	b := newTestBridge(src, pub)

	if err := b.Poll(context.Background()); err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	first := pub.topics()
	// Switch: 4 discovery + 1 state; AP: 3 discovery + 1 state = 9.
	if len(first) != 9 {
		t.Fatalf("first poll published %d messages, want 9: %v", len(first), first)
	}
	if got := countPrefix(first, "homeassistant/"); got != 7 {
		t.Errorf("first poll discovery messages = %d, want 7", got)
	}

	if err := b.Poll(context.Background()); err != nil {
		t.Fatalf("second Poll: %v", err)
	}
	second := pub.topics()[len(first):]
	// Second cycle: no re-announce, only 2 state messages.
	if len(second) != 2 {
		t.Fatalf("second poll published %d messages, want 2: %v", len(second), second)
	}
	if got := countPrefix(second, "homeassistant/"); got != 0 {
		t.Errorf("second poll re-announced %d discovery messages, want 0", got)
	}
}

func TestPollListSitesError(t *testing.T) {
	src := &fakeSource{err: errors.New("no auth")}
	b := newTestBridge(src, &recordingPublisher{})
	if err := b.Poll(context.Background()); err == nil {
		t.Fatal("expected error from Poll when ListSites fails")
	}
}

func TestAnnounceRetriesWhenPublishFails(t *testing.T) {
	src := &fakeSource{
		sites:   []omada.Site{{SiteID: "s1"}},
		devices: map[string][]omada.Device{"s1": {{Mac: "AA-00-00-00-00-01", Type: "ap"}}},
	}
	pub := &recordingPublisher{fail: true}
	b := newTestBridge(src, pub)

	// Publishing fails, so the device must not be marked announced.
	if err := b.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if b.announced["AA-00-00-00-00-01"] {
		t.Fatal("device marked announced despite publish failure")
	}

	// Broker recovers: the next poll should announce (discovery) again.
	pub.fail = false
	if err := b.Poll(context.Background()); err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if got := countPrefix(pub.topics(), "homeassistant/"); got != 3 {
		t.Errorf("after recovery discovery messages = %d, want 3", got)
	}
	if !b.announced["AA-00-00-00-00-01"] {
		t.Error("device not marked announced after successful publish")
	}
}
