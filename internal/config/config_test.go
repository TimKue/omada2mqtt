package config

import "testing"

func setOmada(t *testing.T) {
	t.Helper()
	t.Setenv("OMADA_BASE_URL", "https://controller:8043")
	t.Setenv("OMADA_ID", "omadac-id")
	t.Setenv("OMADA_CLIENT_ID", "client-id")
	t.Setenv("OMADA_CLIENT_SECRET", "secret")
}

func TestFromEnvMissingRequired(t *testing.T) {
	// Only some required vars set; the rest empty.
	t.Setenv("OMADA_BASE_URL", "https://controller:8043")
	t.Setenv("OMADA_ID", "")
	t.Setenv("OMADA_CLIENT_ID", "")
	t.Setenv("OMADA_CLIENT_SECRET", "")

	if _, err := FromEnv(); err == nil {
		t.Fatal("expected error for missing required variables")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	setOmada(t)
	// Leave MQTT unset -> defaults.
	t.Setenv("MQTT_PORT", "")
	t.Setenv("MQTT_TOPIC_PREFIX", "")
	t.Setenv("MQTT_CLIENT_ID", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.MQTTPort != 1883 {
		t.Errorf("MQTTPort = %d, want 1883", cfg.MQTTPort)
	}
	if cfg.MQTTTopicPrefix != "omada2mqtt" {
		t.Errorf("MQTTTopicPrefix = %q, want omada2mqtt", cfg.MQTTTopicPrefix)
	}
	if cfg.MQTTClientID != "omada2mqtt" {
		t.Errorf("MQTTClientID = %q, want omada2mqtt", cfg.MQTTClientID)
	}
	if cfg.InsecureTLS {
		t.Error("InsecureTLS should default to false")
	}
}

func TestFromEnvValues(t *testing.T) {
	setOmada(t)
	t.Setenv("OMADA_INSECURE", "true")
	t.Setenv("MQTT_HOST", "192.168.178.120")
	t.Setenv("MQTT_PORT", "8883")
	t.Setenv("MQTT_USERNAME", "user")
	t.Setenv("MQTT_PASSWORD", "pw")
	t.Setenv("MQTT_TOPIC_PREFIX", "custom")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.InsecureTLS {
		t.Error("InsecureTLS should be true")
	}
	if cfg.MQTTHost != "192.168.178.120" || cfg.MQTTPort != 8883 {
		t.Errorf("MQTT host/port = %s:%d", cfg.MQTTHost, cfg.MQTTPort)
	}
	if cfg.MQTTTopicPrefix != "custom" {
		t.Errorf("MQTTTopicPrefix = %q", cfg.MQTTTopicPrefix)
	}
}

func TestIntEnvFallback(t *testing.T) {
	t.Setenv("SOME_INT", "not-a-number")
	if got := intEnv("SOME_INT", 42); got != 42 {
		t.Errorf("intEnv fallback = %d, want 42", got)
	}
}

func TestBoolEnvFallback(t *testing.T) {
	t.Setenv("SOME_BOOL", "maybe")
	if got := boolEnv("SOME_BOOL", true); got != true {
		t.Errorf("boolEnv fallback = %v, want true", got)
	}
}
