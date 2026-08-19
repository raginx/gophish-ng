package util

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gophish/gophish/models"
)

func buildCSVRequest(csvPayload string) (*http.Request, error) {
	csvHeader := "First Name,Last Name,Email\n"
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("files[]", "example.csv")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write([]byte(csvHeader)); err != nil {
		return nil, err
	}
	if _, err := part.Write([]byte(csvPayload)); err != nil {
		return nil, err
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequest("POST", "http://127.0.0.1", body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r, nil
}

func TestParseCSVEmail(t *testing.T) {
	expected := models.Target{
		BaseRecipient: models.BaseRecipient{
			FirstName: "John",
			LastName:  "Doe",
			Email:     "johndoe@example.com",
		},
	}

	csvPayload := fmt.Sprintf("%s,%s,<%s>", expected.FirstName, expected.LastName, expected.Email)
	r, err := buildCSVRequest(csvPayload)
	if err != nil {
		t.Fatalf("error building CSV request: %v", err)
	}

	got, err := ParseCSV(r)
	if err != nil {
		t.Fatalf("error parsing CSV: %v", err)
	}
	expectedLength := 1
	if len(got) != expectedLength {
		t.Fatalf("invalid number of results received from CSV. expected %d got %d", expectedLength, len(got))
	}
	if !reflect.DeepEqual(expected, got[0]) {
		t.Fatalf("Incorrect targets received. Expected: %#v\nGot: %#v", expected, got)
	}
}

func TestCheckAndCreateSSL(t *testing.T) {
	dir := t.TempDir()
	cp := filepath.Join(dir, "test.crt")
	kp := filepath.Join(dir, "test.key")

	err := CheckAndCreateSSL(cp, kp)
	if err != nil {
		t.Fatalf("error creating self-signed certificate: %v", err)
	}

	// The files should be valid, loadable, non-empty PEM-encoded cert/key
	// files - this exercises the fix where the pem.Encode/Close errors
	// used to be silently ignored, which could otherwise leave behind a
	// truncated cert/key pair while still reporting success.
	if _, err := tls.LoadX509KeyPair(cp, kp); err != nil {
		t.Fatalf("generated certificate/key pair is not valid: %v", err)
	}

	// A second call should be a no-op, since the files already exist.
	certInfo, err := os.Stat(cp)
	if err != nil {
		t.Fatalf("error stat'ing generated certificate: %v", err)
	}
	err = CheckAndCreateSSL(cp, kp)
	if err != nil {
		t.Fatalf("error on second call to CheckAndCreateSSL: %v", err)
	}
	certInfoAfter, err := os.Stat(cp)
	if err != nil {
		t.Fatalf("error stat'ing certificate after second call: %v", err)
	}
	if certInfo.ModTime() != certInfoAfter.ModTime() {
		t.Fatalf("expected existing certificate to be left untouched on second call")
	}
}
