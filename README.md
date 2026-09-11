# omada2mqtt

A small, robust bridge that exposes a **TP-Link Omada** controller to **Home
Assistant** via **MQTT auto-discovery** — written in Go, decoupled from Home
Assistant's release cycle.

It follows the [Zigbee2MQTT] approach: a standalone daemon talks to Omada's
local **Open API**, publishes entities over MQTT discovery, and needs no cloud
account. Everything runs on your own network.

> Status: early but working. The bridge publishes switches and access points to
> Home Assistant with status, load and PoE sensors. Client presence
> (`device_tracker`) and PoE port control are on the roadmap.

## What it does today

For every adopted device (switch / AP / gateway) the bridge creates a Home
Assistant device with:

- **Online** — a connectivity `binary_sensor`
- **CPU** and **Memory** — utilisation sensors (%)
- **PoE Power** — total PoE draw in watts (switches only)

Values refresh on a configurable interval (default 30 s). The bridge publishes a
retained availability topic with an MQTT Last-Will, so entities correctly show
as *unavailable* if the bridge stops.

## How it works

```
Omada controller ──Open API──> omada2mqtt ──MQTT discovery──> Home Assistant
   (local HTTPS)                (this daemon)                  (+ Mosquitto)
```

- `pkg/omada` — a reusable, dependency-free Go client for the Omada Open API
  (OAuth2 client-credentials, token refresh, typed reads).
- `internal/homeassistant` — builds the MQTT discovery configs and state
  payloads (no MQTT dependency, fully unit-tested).
- `internal/bridge` — one poll cycle: read from Omada, publish through a small
  `Publisher` interface (testable with fakes).
- `cmd/omada2mqtt` — wires the Omada client, the MQTT client and the bridge
  together.

## Requirements

- Go 1.24+
- A self-hosted Omada **SDN / Software Controller** with the **Open API**
  enabled (Settings → Platform Integration → Open API). The OC200 hardware
  controller does not support the Open API.
- An Open API application in **Client Mode** (client credentials), role
  *Viewer* is enough for now. Note its **Client ID**, **Client Secret** and
  **Omada ID**.
- An MQTT broker (e.g. the Mosquitto add-on in Home Assistant).

## Configuration

Configuration comes from environment variables, optionally loaded from a
git-ignored `.env` file in the working directory (real environment variables
take precedence). Copy `.env.example` to `.env` and fill it in.

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `OMADA_BASE_URL` | yes | — | Controller URL, e.g. `https://omada.lan:8043` |
| `OMADA_ID` | yes | — | Omada ID from the controller |
| `OMADA_CLIENT_ID` | yes | — | Open API application client ID |
| `OMADA_CLIENT_SECRET` | yes | — | Open API application client secret |
| `OMADA_INSECURE` | no | `false` | Skip TLS verification (self-signed cert) |
| `OMADA_POLL_INTERVAL` | no | `30` | Poll interval in seconds |
| `MQTT_HOST` | yes* | — | MQTT broker host (*required for the bridge) |
| `MQTT_PORT` | no | `1883` | MQTT broker port |
| `MQTT_USERNAME` | no | — | MQTT username |
| `MQTT_PASSWORD` | no | — | MQTT password |
| `MQTT_TOPIC_PREFIX` | no | `omada2mqtt` | Base topic for state messages |
| `MQTT_CLIENT_ID` | no | `omada2mqtt` | MQTT client ID |

## Run

```sh
cp .env.example .env      # then fill in your values
go run ./cmd/omada2mqtt
```

You should see `bridge connected to MQTT ...`, and the devices appear in Home
Assistant under Settings → Devices & Services → MQTT.

There is also a read-only smoke test that authenticates and prints sites,
devices and PoE totals without touching MQTT:

```sh
go run ./cmd/omada-smoke
```

## Development

```sh
make fmt    # format (gofumpt)
make lint   # golangci-lint
make test   # go test -race
make build  # build binaries into ./bin
```

CI runs formatting, `go vet`, tests (with the race detector) and
[golangci-lint] on every push and pull request.

## License

[MIT](LICENSE) © Tim Küppers

[Zigbee2MQTT]: https://www.zigbee2mqtt.io/
[golangci-lint]: https://golangci-lint.run/
