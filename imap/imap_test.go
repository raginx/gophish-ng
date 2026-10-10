package imap

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// startFakeIMAPServer starts a minimal IMAP server that only implements
// enough of the protocol to observe which command a client sends first
// (LOGIN vs AUTHENTICATE) - it always rejects the attempt afterwards, since
// these tests only care about newClient()'s choice of authentication
// command, not a full login round trip. authCaps are the AUTH= capabilities
// the server advertises, defaulting to OAUTHBEARER only.
func startFakeIMAPServer(t *testing.T, authCaps ...string) (addr string, gotCmd <-chan string, cleanup func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("error starting fake IMAP listener: %v", err)
	}
	if len(authCaps) == 0 {
		authCaps = []string{"OAUTHBEARER"}
	}
	capLine := "* CAPABILITY IMAP4rev1"
	for _, c := range authCaps {
		capLine += " AUTH=" + c
	}
	cmdCh := make(chan string, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		// Best effort fake server
		defer func() { _ = conn.Close() }()
		_, _ = conn.Write([]byte("* OK IMAP4rev1 Service Ready\r\n"))
		scanner := bufio.NewScanner(conn)
		firstLineTag := func(line string) string {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				return fields[0]
			}
			return "a1"
		}
		// The client has no CAPABILITY response code in our bare greeting
		// above, so it issues an explicit CAPABILITY command before
		// deciding how to authenticate - answer that first, without
		// advertising SASL-IR, so the actual LOGIN/AUTHENTICATE command
		// we care about is the second line.
		if !scanner.Scan() {
			return
		}
		capTag := firstLineTag(scanner.Text())
		_, _ = conn.Write([]byte(capLine + "\r\n"))
		_, _ = conn.Write([]byte(capTag + " OK CAPABILITY completed\r\n"))

		if !scanner.Scan() {
			return
		}
		line := scanner.Text()
		cmdCh <- line
		_, _ = conn.Write([]byte(firstLineTag(line) + " NO authentication failed (fake server)\r\n"))
	}()
	return l.Addr().String(), cmdCh, func() { _ = l.Close() }
}

func TestNewClientUsesOAuthBearerWhenTokenSet(t *testing.T) {
	addr, gotCmd, cleanup := startFakeIMAPServer(t)
	defer cleanup()

	mbox := &Mailbox{Host: addr, User: "user@example.com", OAuthToken: "test-access-token"}
	// The fake server always rejects the auth attempt, so we only care
	// about what command was sent, not whether newClient() ultimately
	// succeeds.
	_, _ = mbox.newClient()

	select {
	case cmd := <-gotCmd:
		if !strings.Contains(cmd, "AUTHENTICATE OAUTHBEARER") {
			t.Fatalf("expected an AUTHENTICATE OAUTHBEARER command, got %q", cmd)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the client to send a command")
	}
}

// Exchange Online only advertises (and accepts) XOAUTH2
func TestNewClientUsesXOAuth2WhenAdvertised(t *testing.T) {
	for _, caps := range [][]string{{"PLAIN", "XOAUTH2"}, {"XOAUTH2", "OAUTHBEARER"}} {
		addr, gotCmd, cleanup := startFakeIMAPServer(t, caps...)

		mbox := &Mailbox{Host: addr, User: "user@example.com", OAuthToken: "test-access-token"}
		_, _ = mbox.newClient()

		select {
		case cmd := <-gotCmd:
			if !strings.Contains(cmd, "AUTHENTICATE XOAUTH2") {
				t.Fatalf("expected an AUTHENTICATE XOAUTH2 command with caps %v, got %q", caps, cmd)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the client to send a command")
		}
		cleanup()
	}
}

func TestXOAuth2ClientInitialResponse(t *testing.T) {
	c := &xoauth2Client{username: "user@example.com", token: "tok"}
	mech, ir, err := c.Start()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mech != "XOAUTH2" {
		t.Fatalf("expected mechanism XOAUTH2, got %q", mech)
	}
	if expected := "user=user@example.com\x01auth=Bearer tok\x01\x01"; string(ir) != expected {
		t.Fatalf("expected initial response %q, got %q", expected, ir)
	}
	// A failed attempt is answered with an empty response
	resp, err := c.Next([]byte(`{"status":"400"}`))
	if err != nil || len(resp) != 0 {
		t.Fatalf("expected an empty response to an error challenge, got %q, %v", resp, err)
	}
}

func TestNewClientUsesLoginWhenNoToken(t *testing.T) {
	addr, gotCmd, cleanup := startFakeIMAPServer(t)
	defer cleanup()

	mbox := &Mailbox{Host: addr, User: "user@example.com", Pwd: "secret"}
	_, _ = mbox.newClient()

	select {
	case cmd := <-gotCmd:
		if !strings.Contains(cmd, "LOGIN") {
			t.Fatalf("expected a LOGIN command, got %q", cmd)
		}
		if strings.Contains(cmd, "AUTHENTICATE") {
			t.Fatalf("did not expect an AUTHENTICATE command when OAuthToken is unset, got %q", cmd)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the client to send a command")
	}
}
