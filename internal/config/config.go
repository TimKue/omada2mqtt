// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds everything needed to reach one Omada controller and the MQTT
// broker.
type Config struct {
	BaseURL      string // e.g. https://192.168.178.2:8043
	OmadacID     string // "Omada ID" from the controller
	ClientID     string // Open API application client ID
	ClientSecret string // Open API application client secret
	InsecureTLS  bool   // skip cert verification (local self-signed cert)

	// MQTT (optional for the Omada-only smoke test; required for the bridge).
	MQTTHost        string
	MQTTPort        int
	MQTTUsername    string
	MQTTPassword    string
	MQTTTopicPrefix string // base topic, default "omada2mqtt"
	MQTTClientID    string // MQTT client id, default "omada2mqtt"

	// OPNsense (optional; required for the DNS-naming sync).
	OPNsenseURL      string
	OPNsenseKey      string
	OPNsenseSecret   string
	OPNsenseInsecure bool
}

// FromEnv reads the OMADA_* variables and validates the required ones. It first
// loads a .env file from the working directory if one exists (real environment
// variables take precedence over the file).
func FromEnv() (Config, error) {
	loadDotEnv(".env")

	c := Config{
		BaseURL:      strings.TrimSpace(os.Getenv("OMADA_BASE_URL")),
		OmadacID:     strings.TrimSpace(os.Getenv("OMADA_ID")),
		ClientID:     strings.TrimSpace(os.Getenv("OMADA_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("OMADA_CLIENT_SECRET")),
		InsecureTLS:  boolEnv("OMADA_INSECURE", false),

		MQTTHost:        strings.TrimSpace(os.Getenv("MQTT_HOST")),
		MQTTPort:        intEnv("MQTT_PORT", 1883),
		MQTTUsername:    strings.TrimSpace(os.Getenv("MQTT_USERNAME")),
		MQTTPassword:    os.Getenv("MQTT_PASSWORD"),
		MQTTTopicPrefix: envOr("MQTT_TOPIC_PREFIX", "omada2mqtt"),
		MQTTClientID:    envOr("MQTT_CLIENT_ID", "omada2mqtt"),

		OPNsenseURL:      strings.TrimSpace(os.Getenv("OPNSENSE_URL")),
		OPNsenseKey:      strings.TrimSpace(os.Getenv("OPNSENSE_KEY")),
		OPNsenseSecret:   os.Getenv("OPNSENSE_SECRET"),
		OPNsenseInsecure: boolEnv("OPNSENSE_INSECURE", false),
	}

	var missing []string
	for name, v := range map[string]string{
		"OMADA_BASE_URL":      c.BaseURL,
		"OMADA_ID":            c.OmadacID,
		"OMADA_CLIENT_ID":     c.ClientID,
		"OMADA_CLIENT_SECRET": c.ClientSecret,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func envOr(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}

func intEnv(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func boolEnv(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
