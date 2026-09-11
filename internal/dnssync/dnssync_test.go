package dnssync

import (
	"testing"

	"github.com/TimKue/omada2mqtt/pkg/omada"
	"github.com/TimKue/omada2mqtt/pkg/opnsense"
)

func TestNormMAC(t *testing.T) {
	cases := map[string]string{
		"d4:d6:df:0a:63:29": "d4d6df0a6329",
		"D4-D6-DF-0A-63-29": "d4d6df0a6329",
		"D4D6DF0A6329":      "d4d6df0a6329",
	}
	for in, want := range cases {
		if got := NormMAC(in); got != want {
			t.Errorf("NormMAC(%q) = %q, want %q", in, got, want)
		}
	}
	// The two vendor formats must collide.
	if NormMAC("d4:d6:df:0a:63:29") != NormMAC("D4-D6-DF-0A-63-29") {
		t.Error("colon and dash MAC formats should normalise equal")
	}
}

func TestDesiredNames(t *testing.T) {
	leases := []opnsense.Lease{
		{HWAddr: "d4:d6:df:0a:63:29", Hostname: "main-switch"},
		{HWAddr: "aa:bb:cc:00:00:01", Hostname: "*"},   // Dnsmasq placeholder -> skip
		{HWAddr: "aa:bb:cc:00:00:02", Hostname: ""},    // empty -> skip
		{HWAddr: "aa:bb:cc:00:00:03", Hostname: " x "}, // trimmed
	}
	want := DesiredNames(leases)
	if len(want) != 2 {
		t.Fatalf("got %d desired names, want 2: %v", len(want), want)
	}
	if want["d4d6df0a6329"] != "main-switch" {
		t.Errorf("main-switch mapping = %q", want["d4d6df0a6329"])
	}
	if want["aabbcc000003"] != "x" {
		t.Errorf("expected trimmed hostname 'x', got %q", want["aabbcc000003"])
	}
	if _, ok := want["aabbcc000001"]; ok {
		t.Error(`"*" placeholder must not produce a desired name`)
	}
}

func TestPlan(t *testing.T) {
	want := map[string]string{
		"aabbcc000001": "verteilnix", // client currently shows its MAC
		"aabbcc000002": "cfos",       // client has a different name
		"aabbcc000003": "keep",       // client already matches -> no change
		"aabbcc0000ff": "orphan",     // no such client -> ignored
	}
	clients := []omada.ClientInfo{
		{Mac: "AA-BB-CC-00-00-01", Name: "AA-BB-CC-00-00-01"},
		{Mac: "AA-BB-CC-00-00-02", Name: "Wallbox"},
		{Mac: "AA-BB-CC-00-00-03", Name: "keep"},
		{Mac: "AA-BB-CC-00-00-04", Name: "Unmanaged"}, // not in want -> untouched
	}

	plan := Plan("site1", clients, want)
	if len(plan) != 2 {
		t.Fatalf("got %d changes, want 2: %+v", len(plan), plan)
	}

	got := map[string]Change{}
	for _, c := range plan {
		got[c.Mac] = c
		if c.SiteID != "site1" {
			t.Errorf("SiteID = %q", c.SiteID)
		}
	}
	if c := got["AA-BB-CC-00-00-01"]; c.Old != "AA-BB-CC-00-00-01" || c.New != "verteilnix" {
		t.Errorf("unexpected change for 01: %+v", c)
	}
	if c := got["AA-BB-CC-00-00-02"]; c.Old != "Wallbox" || c.New != "cfos" {
		t.Errorf("unexpected change for 02: %+v", c)
	}
	if _, ok := got["AA-BB-CC-00-00-03"]; ok {
		t.Error("client already matching should not be in the plan")
	}
	if _, ok := got["AA-BB-CC-00-00-04"]; ok {
		t.Error("client with no desired name should not be in the plan")
	}
}
