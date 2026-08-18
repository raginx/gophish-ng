package dialer

import (
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestDefaultDeny(t *testing.T) {
	control := restrictedControl([]*net.IPNet{})
	host := "169.254.169.254"
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}
}

func TestDefaultAllow(t *testing.T) {
	control := restrictedControl([]*net.IPNet{})
	host := "1.1.1.1"
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
}

func TestCustomAllow(t *testing.T) {
	host := "127.0.0.1"
	_, ipRange, _ := net.ParseCIDR(fmt.Sprintf("%s/32", host))
	allowed := []*net.IPNet{ipRange}
	control := restrictedControl(allowed)
	conn := new(syscall.RawConn)
	got := control("tcp4", fmt.Sprintf("%s:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
}

func TestCustomDeny(t *testing.T) {
	host := "127.0.0.1"
	_, ipRange, _ := net.ParseCIDR(fmt.Sprintf("%s/32", host))
	allowed := []*net.IPNet{ipRange}
	control := restrictedControl(allowed)
	conn := new(syscall.RawConn)
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	got := control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}
}

func TestSingleIP(t *testing.T) {
	orig := DefaultDialer.AllowedHosts()
	host := "127.0.0.1"
	if err := DefaultDialer.SetAllowedHosts([]string{host}); err != nil {
		t.Fatalf("error setting allowed hosts: %v", err)
	}
	control := DefaultDialer.Dialer().Control
	conn := new(syscall.RawConn)
	expected := fmt.Errorf("upstream connection denied to internal host at %s", host)
	got := control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}

	host = "::1"
	if err := DefaultDialer.SetAllowedHosts([]string{host}); err != nil {
		t.Fatalf("error setting allowed hosts: %v", err)
	}
	control = DefaultDialer.Dialer().Control
	conn = new(syscall.RawConn)
	expected = fmt.Errorf("upstream connection denied to internal host at %s", host)
	got = control("tcp4", "192.168.1.2:80", *conn)
	if !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("unexpected error dialing denylisted host. expected %v got %v", expected, got)
	}

	// Test an allowed connection
	got = control("tcp4", fmt.Sprintf("[%s]:80", host), *conn)
	if got != nil {
		t.Fatalf("error dialing allowed host. got %v", got)
	}
	if err := DefaultDialer.SetAllowedHosts(orig); err != nil {
		t.Fatalf("error restoring allowed hosts: %v", err)
	}
}

// TestAllowedHostsDoesNotBlockExternal guards against a regression where
// allInternal over-matched once any allowed_internal_hosts entry was
// configured (switching the deny list from defaultDeny to allInternal):
//   - "::/0" (default route) matched all of IPv6, not just internal ranges.
//   - "::ffff:0:0/96" (IPv4-mapped IPv6) matched all of IPv4 too, since
//     Go's net.ParseIP represents every parsed IPv4 address internally as
//     a 16-byte IPv4-in-IPv6 value - this range matched plain external
//     IPv4 addresses just as much as an actual IPv4-mapped IPv6 literal.
//
// Both entries let a single allowed_internal_hosts entry deny essentially
// all outbound mail delivery. Covers both address families so a regression
// in either doesn't slip through covering only one.
func TestAllowedHostsDoesNotBlockExternal(t *testing.T) {
	orig := DefaultDialer.AllowedHosts()
	if err := DefaultDialer.SetAllowedHosts([]string{"127.0.0.1"}); err != nil {
		t.Fatalf("error setting allowed hosts: %v", err)
	}
	control := DefaultDialer.Dialer().Control
	conn := new(syscall.RawConn)

	externalIPv4 := "1.1.1.1" // a public IPv4 address
	if got := control("tcp4", fmt.Sprintf("%s:80", externalIPv4), *conn); got != nil {
		t.Fatalf("external IPv4 host incorrectly denied once allowed_internal_hosts was set. got %v", got)
	}

	externalIPv6 := "2606:4700:4700::1111" // a public IPv6 address
	if got := control("tcp6", fmt.Sprintf("[%s]:80", externalIPv6), *conn); got != nil {
		t.Fatalf("external IPv6 host incorrectly denied once allowed_internal_hosts was set. got %v", got)
	}

	// An actual internal host should still be denied.
	if got := control("tcp4", "127.0.0.2:80", *conn); got == nil || !strings.Contains(got.Error(), "upstream connection denied") {
		t.Fatalf("internal host should still be denied once allowed_internal_hosts was set. got %v", got)
	}

	if err := DefaultDialer.SetAllowedHosts(orig); err != nil {
		t.Fatalf("error restoring allowed hosts: %v", err)
	}
}
