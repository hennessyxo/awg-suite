// Package shaper builds and applies Linux `tc` traffic-control rules to cap a
// client's bandwidth (instead of cutting it off entirely).
//
// Download (traffic the server sends out the VPN interface toward a client) is
// shaped with an HTB class per client IP. Upload (traffic arriving from a
// client) is rate-limited with an ingress policer. The command-planning logic
// is pure and unit-tested; Apply is a thin shell-out to `tc`.
package shaper

import (
	"fmt"
	"os/exec"
	"strings"
)

// linkMbit is the assumed interface line rate; unshaped traffic may burst to it.
const linkMbit = 1000

// Limit is a per-client bandwidth cap.
type Limit struct {
	Octet int // host octet in the VPN subnet (e.g. 2 → 10.66.66.2)
	Mbit  int // cap in Mbit/s (must be > 0)
}

// Command is a single `tc` invocation. IgnoreError marks teardown commands that
// are expected to fail when there is nothing to remove yet.
type Command struct {
	Args        []string
	IgnoreError bool
}

// Plan returns the ordered `tc` commands that make the live shaping match the
// desired set of limits. It is idempotent: it tears the trees down and rebuilds
// them, so applying the same plan twice is safe.
//
// base is the subnet prefix, e.g. "10.66.66.".
func Plan(iface, base string, limits []Limit) []Command {
	delEgress := Command{Args: []string{"qdisc", "del", "dev", iface, "root"}, IgnoreError: true}
	delIngress := Command{Args: []string{"qdisc", "del", "dev", iface, "ingress"}, IgnoreError: true}

	// No limits → just clear any existing shaping.
	if len(limits) == 0 {
		return []Command{delEgress, delIngress}
	}

	cmds := []Command{
		delEgress,
		// Egress HTB tree: default class 9999 may burst to the full link rate.
		{Args: []string{"qdisc", "add", "dev", iface, "root", "handle", "1:", "htb", "default", "9999"}},
		{Args: []string{"class", "add", "dev", iface, "parent", "1:", "classid", "1:1", "htb", "rate", mbit(linkMbit)}},
		{Args: []string{"class", "add", "dev", iface, "parent", "1:1", "classid", "1:9999", "htb", "rate", mbit(1), "ceil", mbit(linkMbit)}},
	}

	for _, l := range limits {
		cid := classID(l.Octet)
		ip := base + itoa(l.Octet)
		cmds = append(cmds,
			Command{Args: []string{"class", "add", "dev", iface, "parent", "1:1", "classid", "1:" + itoa(cid), "htb", "rate", mbit(l.Mbit), "ceil", mbit(l.Mbit)}},
			Command{Args: []string{"filter", "add", "dev", iface, "protocol", "ip", "parent", "1:", "prio", "1", "u32", "match", "ip", "dst", ip + "/32", "flowid", "1:" + itoa(cid)}},
		)
	}

	// Ingress policer for upload caps.
	cmds = append(cmds,
		delIngress,
		Command{Args: []string{"qdisc", "add", "dev", iface, "handle", "ffff:", "ingress"}},
	)
	for _, l := range limits {
		ip := base + itoa(l.Octet)
		cmds = append(cmds, Command{Args: []string{
			"filter", "add", "dev", iface, "parent", "ffff:", "protocol", "ip", "prio", "1",
			"u32", "match", "ip", "src", ip + "/32",
			"police", "rate", mbit(l.Mbit), "burst", burst(l.Mbit), "drop", "flowid", ":1",
		}})
	}
	return cmds
}

// Apply runs the planned commands with `tc`, ignoring failures of teardown
// commands. It returns the first real error encountered.
func Apply(cmds []Command) error {
	for _, c := range cmds {
		if err := exec.Command("tc", c.Args...).Run(); err != nil && !c.IgnoreError {
			return fmt.Errorf("tc %s: %w", strings.Join(c.Args, " "), err)
		}
	}
	return nil
}

// classID maps a host octet to a unique HTB class id that avoids the reserved
// 1 (root) and 9999 (default) ids.
func classID(octet int) int { return octet + 10 }

func mbit(n int) string { return itoa(n) + "mbit" }

// burst sizes the ingress policer bucket at roughly 50 ms of the rate.
func burst(m int) string {
	kb := m * 16
	if kb < 32 {
		kb = 32
	}
	return itoa(kb) + "k"
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
