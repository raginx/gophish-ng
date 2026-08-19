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
// command, not a full login round trip.
func startFakeIMAPServer(t *testing.T) (addr string, gotCmd <-chan string, cleanup func()) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("error starting fake IMAP listener: %v", err)
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
		_, _ = conn.Write([]byte("* CAPABILITY IMAP4rev1 AUTH=OAUTHBEARER\r\n"))
		_, _ = conn.Write([]byte(capTag + " OK CAPABILITY completed\r\n"))

		if !scanner.Scan() {
			return
		}
		line := scanner.Text()
		cmdCh <- line
		_, _ = conn.Write([]byte(firstLineTag(line) + " NO authentication failed (fake server)\r\n"))
	}()
	return l.Addr().String(), cmdCh, func() { l.Close() }
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
