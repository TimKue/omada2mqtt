// Package homeassistant builds the MQTT topics and payloads for Home
// Assistant's MQTT auto-discovery from Omada device state. It has no MQTT
// dependency of its own: it turns an omada.Device into a set of retained
// discovery configs and a periodic state message, which the caller publishes.
//
// See https://www.home-assistant.io/integrations/mqtt/#mqtt-discovery.
package homeassistant

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TimKue/omada2mqtt/pkg/omada"
)

// DefaultDiscoveryPrefix is Home Assistant's default discovery topic prefix.
const DefaultDiscoveryPrefix = "homeassistant"

// Message is a single MQTT publish: a topic, a payload and the retain flag.
type Message struct {
	Topic   string
	Payload []byte
	Retain  bool
}

// Builder produces discovery and state messages for one bridge instance.
// The zero value is not usable; construct it with New.
type Builder struct {
	topicPrefix     string
	discoveryPrefix string
	availTopic      string
}

// New returns a Builder. topicPrefix is the bridge's base topic (e.g.
// "omada2mqtt"), availTopic is the bridge availability topic, and
// discoveryPrefix is Home Assistant's discovery prefix (empty ->
// DefaultDiscoveryPrefix).
func New(topicPrefix, availTopic, discoveryPrefix string) *Builder {
	if discoveryPrefix == "" {
		discoveryPrefix = DefaultDiscoveryPrefix
	}
	return &Builder{
		topicPrefix:     topicPrefix,
		discoveryPrefix: discoveryPrefix,
		availTopic:      availTopic,
	}
}

// Slug turns a MAC into a topic/id-safe token, e.g. "d4d6df0a6329".
func Slug(mac string) string {
	return strings.ToLower(strings.NewReplacer("-", "", ":", "").Replace(mac))
}

// StateTopic is the topic a device's state JSON is published to.
func (b *Builder) StateTopic(mac string) string {
	return fmt.Sprintf("%s/%s/state", b.topicPrefix, Slug(mac))
}

type haDevice struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	SWVersion    string   `json:"sw_version,omitempty"`
}

type discovery struct {
	Name              string   `json:"name"`
	UniqueID          string   `json:"unique_id"`
	StateTopic        string   `json:"state_topic"`
	ValueTemplate     string   `json:"value_template"`
	DeviceClass       string   `json:"device_class,omitempty"`
	StateClass        string   `json:"state_class,omitempty"`
	UnitOfMeasurement string   `json:"unit_of_measurement,omitempty"`
	Icon              string   `json:"icon,omitempty"`
	PayloadOn         string   `json:"payload_on,omitempty"`
	PayloadOff        string   `json:"payload_off,omitempty"`
	AvailabilityTopic string   `json:"availability_topic"`
	PayloadAvailable  string   `json:"payload_available"`
	PayloadNotAvail   string   `json:"payload_not_available"`
	Device            haDevice `json:"device"`
}

func (b *Builder) configTopic(component, node, key string) string {
	return fmt.Sprintf("%s/%s/%s/%s/config", b.discoveryPrefix, component, node, key)
}

// Discovery returns the retained discovery-config messages for one device:
// an Online binary_sensor plus CPU and Memory sensors, and — for switches — a
// PoE Power sensor. The messages are marshaled; a marshal error is impossible
// for these fixed structs and is reported so the caller can log it.
func (b *Builder) Discovery(d omada.Device) ([]Message, error) {
	slug := Slug(d.Mac)
	uid := "omada2mqtt_" + slug
	dev := haDevice{
		Identifiers:  []string{uid},
		Name:         d.Name,
		Manufacturer: "TP-Link Omada",
		Model:        d.Model,
		SWVersion:    d.FirmwareVersion,
	}
	base := discovery{
		StateTopic:        b.StateTopic(d.Mac),
		AvailabilityTopic: b.availTopic,
		PayloadAvailable:  "online",
		PayloadNotAvail:   "offline",
		Device:            dev,
	}

	type entity struct {
		component string
		key       string
		cfg       discovery
	}
	entities := make([]entity, 0, 4)

	online := base
	online.Name = "Online"
	online.UniqueID = uid + "_online"
	online.DeviceClass = "connectivity"
	online.ValueTemplate = "{{ value_json.online }}"
	online.PayloadOn = "ON"
	online.PayloadOff = "OFF"
	entities = append(entities, entity{"binary_sensor", "online", online})

	cpu := base
	cpu.Name = "CPU"
	cpu.UniqueID = uid + "_cpu"
	cpu.ValueTemplate = "{{ value_json.cpu }}"
	cpu.UnitOfMeasurement = "%"
	cpu.StateClass = "measurement"
	cpu.Icon = "mdi:cpu-64-bit"
	entities = append(entities, entity{"sensor", "cpu", cpu})

	mem := base
	mem.Name = "Memory"
	mem.UniqueID = uid + "_mem"
	mem.ValueTemplate = "{{ value_json.mem }}"
	mem.UnitOfMeasurement = "%"
	mem.StateClass = "measurement"
	mem.Icon = "mdi:memory"
	entities = append(entities, entity{"sensor", "mem", mem})

	if d.Type == "switch" {
		poe := base
		poe.Name = "PoE Power"
		poe.UniqueID = uid + "_poe"
		poe.ValueTemplate = "{{ value_json.poe_watts }}"
		poe.UnitOfMeasurement = "W"
		poe.DeviceClass = "power"
		poe.StateClass = "measurement"
		entities = append(entities, entity{"sensor", "poe", poe})
	}

	msgs := make([]Message, 0, len(entities))
	for _, e := range entities {
		payload, err := json.Marshal(e.cfg)
		if err != nil {
			return nil, fmt.Errorf("marshal discovery %s/%s: %w", e.component, e.key, err)
		}
		msgs = append(msgs, Message{
			Topic:   b.configTopic(e.component, slug, e.key),
			Payload: payload,
			Retain:  true,
		})
	}
	return msgs, nil
}

// State returns the retained state message for a device. poeWatts is the
// summed PoE draw for switches and is ignored for other device types.
func (b *Builder) State(d omada.Device, poeWatts float64) (Message, error) {
	online := "OFF"
	if d.Online() {
		online = "ON"
	}
	state := map[string]any{
		"online": online,
		"cpu":    d.CPUUtil,
		"mem":    d.MemUtil,
	}
	if d.Type == "switch" {
		state["poe_watts"] = poeWatts
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return Message{}, fmt.Errorf("marshal state for %s: %w", d.Mac, err)
	}
	return Message{Topic: b.StateTopic(d.Mac), Payload: payload, Retain: true}, nil
}
