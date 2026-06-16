package shaper

import (
	"strings"
	"testing"
)

// joined flattens a plan into one searchable string per command.
func joined(cmds []Command) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = strings.Join(c.Args, " ")
	}
	return out
}

func contains(lines []string, substr string) bool {
	for _, l := range lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func TestPlan_NoLimitsClearsShaping(t *testing.T) {
	cmds := Plan("awg0", "10.66.66.", nil)
	if len(cmds) != 2 {
		t.Fatalf("empty plan = %d commands, want 2 (del root, del ingress)", len(cmds))
	}
	for _, c := range cmds {
		if !c.IgnoreError {
			t.Errorf("teardown command should ignore errors: %v", c.Args)
		}
	}
	lines := joined(cmds)
	if !contains(lines, "qdisc del dev awg0 root") || !contains(lines, "qdisc del dev awg0 ingress") {
		t.Errorf("empty plan missing teardown commands: %v", lines)
	}
}

func TestPlan_BuildsEgressAndIngressForClient(t *testing.T) {
	cmds := Plan("awg0", "10.66.66.", []Limit{{Octet: 2, Mbit: 10}})
	lines := joined(cmds)

	wants := []string{
		"qdisc add dev awg0 root handle 1: htb default 9999",
		"class add dev awg0 parent 1:1 classid 1:12 htb rate 10mbit ceil 10mbit", // octet 2 → classid 12
		"match ip dst 10.66.66.2/32 flowid 1:12",
		"qdisc add dev awg0 handle ffff: ingress",
		"match ip src 10.66.66.2/32",
		"police rate 10mbit",
	}
	for _, w := range wants {
		if !contains(lines, w) {
			t.Errorf("plan missing %q\n%s", w, strings.Join(lines, "\n"))
		}
	}
}

func TestPlan_DistinctClassIDsPerClient(t *testing.T) {
	cmds := Plan("awg0", "10.66.66.", []Limit{{Octet: 2, Mbit: 5}, {Octet: 3, Mbit: 20}})
	lines := joined(cmds)
	if !contains(lines, "classid 1:12 htb rate 5mbit") {
		t.Error("client octet 2 should map to classid 1:12 @ 5mbit")
	}
	if !contains(lines, "classid 1:13 htb rate 20mbit") {
		t.Error("client octet 3 should map to classid 1:13 @ 20mbit")
	}
}

func TestClassID_AvoidsReservedIDs(t *testing.T) {
	// octet-derived ids must never collide with 1 (root) or 9999 (default)
	for _, octet := range []int{2, 254} {
		if id := classID(octet); id == 1 || id == 9999 {
			t.Errorf("classID(%d) = %d collides with a reserved id", octet, id)
		}
	}
}
