// Command omada2mqtt is the bridge: it polls the Omada controller and publishes
// device state to MQTT with Home Assistant auto-discovery, so switches and APs
// show up in Home Assistant automatically.
//
//	go run ./cmd/omada2mqtt
//
// Configuration comes from the environment / .env (see .env.example).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/TimKue/omada2mqtt/internal/bridge"
	"github.com/TimKue/omada2mqtt/internal/config"
	"github.com/TimKue/omada2mqtt/internal/homeassistant"
	"github.com/TimKue/omada2mqtt/pkg/omada"
)

const (
	defaultInterval = 30 * time.Second
	publishTimeout  = 5 * time.Second
	connectTimeout  = 10 * time.Second
)

// Build information, set via -ldflags at release time (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.SetFlags(log.LstdFlags)
	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	log.Printf("omada2mqtt %s (commit %s, built %s)", version, commit, date)

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if cfg.MQTTHost == "" {
		return fmt.Errorf("MQTT_HOST is not set (fill the MQTT_* values in .env)")
	}

	interval := pollInterval()

	opts := []omada.Option{}
	if cfg.InsecureTLS {
		opts = append(opts, omada.WithInsecureTLS())
	}
	oc := omada.NewClient(cfg.BaseURL, cfg.OmadacID, cfg.ClientID, cfg.ClientSecret, opts...)

	availTopic := cfg.MQTTTopicPrefix + "/bridge/availability"

	mc, err := connectMQTT(cfg, availTopic)
	if err != nil {
		return err
	}
	defer mc.Disconnect(250)

	pub := &mqttPublisher{c: mc, timeout: publishTimeout}
	if err := pub.Publish(availTopic, []byte("online"), true); err != nil {
		log.Printf("publish availability: %v", err)
	}
	log.Printf("bridge connected to MQTT %s:%d, polling every %s", cfg.MQTTHost, cfg.MQTTPort, interval)

	ha := homeassistant.New(cfg.MQTTTopicPrefix, availTopic, "")
	b := bridge.New(oc, pub, ha, log.Printf)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := b.Poll(ctx); err != nil {
		log.Printf("poll: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := pub.Publish(availTopic, []byte("offline"), true); err != nil {
				log.Printf("publish availability: %v", err)
			}
			log.Print("shutting down")
			return nil
		case <-ticker.C:
			if err := b.Poll(ctx); err != nil {
				log.Printf("poll: %v", err)
			}
		}
	}
}

func pollInterval() time.Duration {
	if v := strings.TrimSpace(os.Getenv("OMADA_POLL_INTERVAL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultInterval
}

// connectMQTT builds an MQTT client with a Last-Will so entities go
// unavailable if the bridge crashes, and blocks until connected.
func connectMQTT(cfg config.Config, availTopic string) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions().
		AddBroker(fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)).
		SetClientID(cfg.MQTTClientID).
		SetUsername(cfg.MQTTUsername).
		SetPassword(cfg.MQTTPassword).
		SetConnectTimeout(connectTimeout).
		SetAutoReconnect(true).
		SetWill(availTopic, "offline", 1, true)

	mc := mqtt.NewClient(opts)
	t := mc.Connect()
	if !t.WaitTimeout(connectTimeout) {
		return nil, fmt.Errorf("connect MQTT %s:%d: timeout", cfg.MQTTHost, cfg.MQTTPort)
	}
	if err := t.Error(); err != nil {
		return nil, fmt.Errorf("connect MQTT %s:%d: %w", cfg.MQTTHost, cfg.MQTTPort, err)
	}
	return mc, nil
}

// mqttPublisher adapts a paho MQTT client to the bridge.Publisher interface.
type mqttPublisher struct {
	c       mqtt.Client
	timeout time.Duration
}

func (p *mqttPublisher) Publish(topic string, payload []byte, retain bool) error {
	t := p.c.Publish(topic, 1, retain, payload)
	if !t.WaitTimeout(p.timeout) {
		return fmt.Errorf("timeout")
	}
	return t.Error()
}
