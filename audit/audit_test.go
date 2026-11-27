package audit

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// readLines returns the non-empty lines of a file.
func readLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			lines = append(lines, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}

func TestRecordWritesOneJSONLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := Setup(&Config{Enabled: true, Path: path}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() { _ = Close() }()

	Record(Event{
		Event:    EventLoginSuccess,
		Outcome:  OutcomeSuccess,
		Actor:    &Actor{ID: 1, Username: "admin"},
		SourceIP: "203.0.113.4",
	})

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), lines)
	}
	var e Event
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("event is not valid JSON: %v", err)
	}
	if e.Event != EventLoginSuccess || e.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected event/outcome: %+v", e)
	}
	if e.Actor == nil || e.Actor.ID != 1 || e.Actor.Username != "admin" {
		t.Fatalf("unexpected actor: %+v", e.Actor)
	}
	if e.SourceIP != "203.0.113.4" {
		t.Fatalf("unexpected source_ip: %q", e.SourceIP)
	}
	if e.Time.IsZero() {
		t.Fatal("Record should have stamped a time")
	}
}

func TestDisabledSinkIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := Setup(&Config{Enabled: false, Path: path}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() { _ = Close() }()

	Record(Event{Event: EventLoginSuccess, Outcome: OutcomeSuccess})

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled sink must not create the file, stat err = %v", err)
	}
}

// TestRecordIsInjectionSafe proves that an untrusted username containing
// newlines and control characters cannot forge extra log lines
func TestRecordIsInjectionSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := Setup(&Config{Enabled: true, Path: path}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() { _ = Close() }()

	malicious := "evil\n{\"event\":\"login_success\",\"outcome\":\"success\"}\n"
	Record(Event{
		Event:   EventLoginFailure,
		Outcome: OutcomeFailure,
		Reason:  ReasonUnknownUser,
		Actor:   &Actor{Username: malicious},
	})

	lines := readLines(t, path)
	if len(lines) != 1 {
		t.Fatalf("injection produced %d lines, want exactly 1: %q", len(lines), lines)
	}
	var e Event
	if err := json.Unmarshal([]byte(lines[0]), &e); err != nil {
		t.Fatalf("event is not valid JSON: %v", err)
	}
	if e.Actor == nil || e.Actor.Username != malicious {
		t.Fatalf("username not preserved verbatim in its field: %+v", e.Actor)
	}
}

func TestRecordConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	if err := Setup(&Config{Enabled: true, Path: path}); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	defer func() { _ = Close() }()

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			Record(Event{Event: EventUserModified, Outcome: OutcomeSuccess})
		}()
	}
	wg.Wait()

	lines := readLines(t, path)
	if len(lines) != n {
		t.Fatalf("got %d lines, want %d", len(lines), n)
	}
	for i, l := range lines {
		var e Event
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("line %d is not valid JSON (interleaved write?): %v", i, err)
		}
	}
}

func TestSourceIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.9:54321"
	if got := SourceIP(r); got != "198.51.100.9" {
		t.Fatalf("SourceIP = %q, want 198.51.100.9", got)
	}
	// A RemoteAddr without a port falls back to the raw value.
	r.RemoteAddr = "198.51.100.9"
	if got := SourceIP(r); got != "198.51.100.9" {
		t.Fatalf("SourceIP = %q, want 198.51.100.9", got)
	}
	if got := SourceIP(nil); got != "" {
		t.Fatalf("SourceIP(nil) = %q, want empty", got)
	}
}
