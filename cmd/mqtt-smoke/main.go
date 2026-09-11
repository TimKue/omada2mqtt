// Command mqtt-smoke is a connectivity test for the MQTT broker: it connects
// using the MQTT_* settings and publishes one retained test message.
//
//	go get github.com/eclipse/paho.mqtt.golang
//	# fill MQTT_* in .env, then:
//	go run ./cmd/mqtt-smoke
//
// Success: it prints "connected" and "published"; check the topic
// "<prefix>/test" in your broker (e.g. Home Assistant MQTT / mosquitto_sub).
package main

import (
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/TimKue/omada2mqtt/internal/config"
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
	if cfg.MQTTHost == "" {
		return fmt.Errorf("MQTT_HOST is not set (fill the MQTT_* values in .env)")
	}

	broker := fmt.Sprintf("tcp://%s:%d", cfg.MQTTHost, cfg.MQTTPort)
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID(cfg.MQTTClientID).
		SetUsername(cfg.MQTTUsername).
		SetPassword(cfg.MQTTPassword).
		SetConnectTimeout(10 * time.Second)

	client := mqtt.NewClient(opts)
	tok := client.Connect()
	if !tok.WaitTimeout(10 * time.Second) {
		return fmt.Errorf("connect to %s timed out", broker)
	}
	if err := tok.Error(); err != nil {
		return fmt.Errorf("connect to %s: %w", broker, err)
	}
	fmt.Printf("connected to %s\n", broker)
	defer client.Disconnect(250)

	topic := cfg.MQTTTopicPrefix + "/test"
	payload := fmt.Sprintf("hello from omada2mqtt at %s", time.Now().Format(time.RFC3339))
	pub := client.Publish(topic, 0, true, payload)
	if !pub.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("publish to %s timed out", topic)
	}
	if err := pub.Error(); err != nil {
		return fmt.Errorf("publish to %s: %w", topic, err)
	}
	fmt.Printf("published retained message to %s\n", topic)
	return nil
}
