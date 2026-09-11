package opnsense

// Lease is one Dnsmasq DHCP lease as returned by /api/dnsmasq/leases/search.
// Field names are best-effort and confirmed against the live firewall via the
// RawLeases dump; extend once verified.
type Lease struct {
	HWAddr   string `json:"hwaddr"`
	Address  string `json:"address"`
	Hostname string `json:"hostname"`
}

// leaseSearch is the envelope the OPNsense "search" endpoints return.
type leaseSearch struct {
	Rows     []Lease `json:"rows"`
	RowCount int     `json:"rowCount"`
	Total    int     `json:"total"`
}
