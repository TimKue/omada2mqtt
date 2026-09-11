package homeassistant

import (
	"encoding/json"
	"testing"

	"github.com/TimKue/omada2mqtt/pkg/omada"
)

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"D4-D6-DF-0A-63-29": "d4d6df0a6329",
		"d4:d6:df:0a:63:29": "d4d6df0a6329",
		"AABBCC":            "aabbcc",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStateTopic(t *testing.T) {
	b := New("omada2mqtt", "omada2mqtt/bridge/availability", "")
	if got, want := b.StateTopic("AA-BB-CC-DD-EE-FF"), "omada2mqtt/aabbccddeeff/state"; got != want {
		t.Errorf("StateTopic = %q, want %q", got, want)
	}
}

func TestDiscoverySwitchHasPoE(t *testing.T) {
	b := New("omada2mqtt", "omada2mqtt/bridge/availability", "")
	sw := omada.Device{Mac: "AA-BB-CC-00-00-01", Name: "Core", Type: "switch", Model: "ES220GP"}
	msgs, err := b.Discovery(sw)
	if err != nil {
		t.Fatalf("Discovery: %v", err)
	}
	// Online + CPU + Memory + PoE = 4 entities.
	if len(msgs) != 4 {
		t.Fatalf("switch: got %d discovery messages, want 4", len(msgs))
	}

	wantTopics := map[string]bool{
		"homeassistant/binary_sensor/aabbcc000001/online/config": false,
		"homeassistant/sensor/aabbcc000001/cpu/config":           false,
		"homeassistant/sensor/aabbcc000001/mem/config":           false,
		"homeassistant/sensor/aabbcc000001/poe/config":           false,
	}
	for _, m := range msgs {
		if !m.Retain {
			t.Errorf("discovery message %s not retained", m.Topic)
		}
		if _, ok := wantTopics[m.Topic]; !ok {
			t.Errorf("unexpected discovery topic %q", m.Topic)
			continue
		}
		wantTopics[m.Topic] = true
	}
	for topic, seen := range wantTopics {
		if !seen {
			t.Errorf("missing discovery topic %q", topic)
		}
	}
}

func TestDiscoveryAPHasNoPoE(t *testing.T) {
	b := New("omada2mqtt", "omada2mqtt/bridge/availability", "")
	ap := omada.Device{Mac: "AA-BB-CC-00-00-02", Name: "AP", Type: "ap", Model: "EAP650"}
	msgs, err := b.Discovery(ap)
	if err != nil {
		t.Fatalf("Discovery: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("ap: got %d discovery messages, want 3", len(msgs))
	}
	for _, m := range msgs {
		if m.Topic == "homeassistant/sensor/aabbcc000002/poe/config" {
			t.Errorf("AP should not get a PoE entity")
		}
	}
}

func TestDiscoveryPayloadFields(t *testing.T) {
	b := New("omada2mqtt", "omada2mqtt/bridge/availability", "")
	sw := omada.Device{Mac: "AA-BB-CC-00-00-01", Name: "Core", Type: "switch", Model: "ES220GP", FirmwareVersion: "1.2.3"}
	msgs, err := b.Discovery(sw)
	if err != nil {
		t.Fatalf("Discovery: %v", err)
	}
	var poe map[string]any
	for _, m := range msgs {
		if m.Topic == "homeassistant/sensor/aabbcc000001/poe/config" {
			if err := json.Unmarshal(m.Payload, &poe); err != nil {
				t.Fatalf("unmarshal poe config: %v", err)
			}
		}
	}
	if poe == nil {
		t.Fatal("no PoE config found")
	}
	if poe["unique_id"] != "omada2mqtt_aabbcc000001_poe" {
		t.Errorf("unique_id = %v", poe["unique_id"])
	}
	if poe["device_class"] != "power" || poe["unit_of_measurement"] != "W" {
		t.Errorf("device_class/unit = %v/%v", poe["device_class"], poe["unit_of_measurement"])
	}
	dev, ok := poe["device"].(map[string]any)
	if !ok {
		t.Fatalf("device block missing/wrong type: %T", poe["device"])
	}
	if dev["sw_version"] != "1.2.3" || dev["manufacturer"] != "TP-Link Omada" {
		t.Errorf("device block = %v", dev)
	}
}

func TestState(t *testing.T) {
	b := New("omada2mqtt", "omada2mqtt/bridge/availability", "")

	sw := omada.Device{Mac: "AA-BB-CC-00-00-01", Type: "switch", Status: 1, CPUUtil: 12, MemUtil: 34}
	m, err := b.State(sw, 7.3)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if !m.Retain {
		t.Error("state should be retained")
	}
	var s map[string]any
	if err := json.Unmarshal(m.Payload, &s); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if s["online"] != "ON" {
		t.Errorf("online = %v, want ON", s["online"])
	}
	if s["cpu"].(float64) != 12 || s["mem"].(float64) != 34 {
		t.Errorf("cpu/mem = %v/%v", s["cpu"], s["mem"])
	}
	if s["poe_watts"].(float64) != 7.3 {
		t.Errorf("poe_watts = %v, want 7.3", s["poe_watts"])
	}

	// An offline AP: online OFF and no poe_watts key.
	ap := omada.Device{Mac: "AA-BB-CC-00-00-02", Type: "ap", Status: 0}
	m, err = b.State(ap, 0)
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	s = nil
	if err := json.Unmarshal(m.Payload, &s); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if s["online"] != "OFF" {
		t.Errorf("online = %v, want OFF", s["online"])
	}
	if _, ok := s["poe_watts"]; ok {
		t.Errorf("AP state should not contain poe_watts")
	}
}
