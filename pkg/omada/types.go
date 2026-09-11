package omada

// apiEnvelope is the common response wrapper the Omada Open API puts around
// every payload. errorCode == 0 means success; anything else is an API-level
// error and msg carries the human-readable reason.
type apiEnvelope[T any] struct {
	ErrorCode int    `json:"errorCode"`
	Msg       string `json:"msg"`
	Result    T      `json:"result"`
}

// tokenResult is the payload of a successful token request.
type tokenResult struct {
	AccessToken  string `json:"accessToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int    `json:"expiresIn"` // seconds
	RefreshToken string `json:"refreshToken"`
}

// page is the Omada pagination wrapper used by list endpoints.
type page[T any] struct {
	TotalRows   int `json:"totalRows"`
	CurrentPage int `json:"currentPage"`
	CurrentSize int `json:"currentSize"`
	Data        []T `json:"data"`
}

// Site is a minimal view of an Omada site. Extend as needed once we confirm
// the exact fields against the controller's built-in "Online API Document".
type Site struct {
	SiteID string `json:"siteId"`
	Name   string `json:"name"`
	Region string `json:"region,omitempty"`
	Type   int    `json:"type,omitempty"`
}

// Device is an adopted Omada device (AP / switch / gateway) as returned by the
// device-list endpoint. PoE consumption and per-port data are NOT in this list
// (they live in the per-device detail endpoint) and come in a later slice.
type Device struct {
	Name            string `json:"name"`
	Type            string `json:"type"` // "ap" | "switch" | "gateway"
	Mac             string `json:"mac"`
	Model           string `json:"model,omitempty"`
	ModelName       string `json:"modelName,omitempty"`
	IP              string `json:"ip,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	SN              string `json:"sn,omitempty"`
	Status          int    `json:"status"` // 1 = connected
	Uptime          string `json:"uptime,omitempty"`
	CPUUtil         int    `json:"cpuUtil"`
	MemUtil         int    `json:"memUtil"`
	LastSeen        int64  `json:"lastSeen,omitempty"` // epoch milliseconds
}

// Online reports whether the device is currently connected.
func (d Device) Online() bool { return d.Status == 1 }

// SwitchPort is one switch port's PoE-relevant state, taken from the site-wide
// switches/ports/poe-info grid. PoeWatts is the live draw (0 when the port is
// not delivering PoE); non-PoE ports (SFP/uplink) report SupportPoe=false.
type SwitchPort struct {
	SwitchMac  string  `json:"switchMac"`
	SwitchName string  `json:"switchName"`
	Port       int     `json:"port"`
	Name       string  `json:"portName"`
	SupportPoe bool    `json:"supportPoe"`
	PoeWatts   float64 `json:"power"`   // live PoE power draw, watts
	PoeVolts   float64 `json:"voltage"` // volts
	PoeMilliA  float64 `json:"current"` // milliamps
}
