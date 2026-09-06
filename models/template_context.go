package models

import (
	"bytes"
	"fmt"
	"net/mail"
	"net/url"
	"path"
	"regexp"
	"strings"
	"text/template"
)

// TemplateContext is an interface that allows both campaigns and email
// requests to have a PhishingTemplateContext generated for them.
type TemplateContext interface {
	getFromAddress() string
	getBaseURL() string
}

// PhishingTemplateContext is the context that is sent to any template, such
// as the email or landing page content.
type PhishingTemplateContext struct {
	From        string
	URL         string
	Tracker     string
	TrackingURL string
	RId         string
	BaseURL     string
	// Domain is the portion of the recipient's Email after the "@", e.g.
	// "example.com" for "foo@example.com". Empty if the email has no "@".
	Domain string
	BaseRecipient
}

// NewPhishingTemplateContext returns a populated PhishingTemplateContext,
// parsing the correct fields from the provided TemplateContext and recipient.
func NewPhishingTemplateContext(ctx TemplateContext, r BaseRecipient, rid string) (PhishingTemplateContext, error) {
	f, err := mail.ParseAddress(ctx.getFromAddress())
	if err != nil {
		return PhishingTemplateContext{}, err
	}
	fn := f.Name
	if fn == "" {
		fn = f.Address
	}
	templateURL, err := ExecuteTemplate(ctx.getBaseURL(), r)
	if err != nil {
		return PhishingTemplateContext{}, err
	}

	// For the base URL, we'll reset the the path and the query
	// This will create a URL in the form of http://example.com
	baseURL, err := url.Parse(templateURL)
	if err != nil {
		return PhishingTemplateContext{}, err
	}
	baseURL.Path = ""
	baseURL.RawQuery = ""

	phishURL, _ := url.Parse(templateURL)
	q := phishURL.Query()
	q.Set(RecipientParameter, rid)
	phishURL.RawQuery = q.Encode()

	trackingURL, _ := url.Parse(templateURL)
	trackingURL.Path = path.Join(trackingURL.Path, "/track")
	trackingURL.RawQuery = q.Encode()

	domain := ""
	if at := strings.LastIndex(r.Email, "@"); at >= 0 {
		domain = r.Email[at+1:]
	}

	return PhishingTemplateContext{
		BaseRecipient: r,
		BaseURL:       baseURL.String(),
		URL:           phishURL.String(),
		TrackingURL:   trackingURL.String(),
		Tracker:       "<img alt='' style='display: none' src='" + trackingURL.String() + "'/>",
		From:          fn,
		RId:           rid,
		Domain:        domain,
	}, nil
}

// ExecuteTemplate creates a templated string based on the provided
// template body and data.
func ExecuteTemplate(text string, data interface{}) (string, error) {
	buff := bytes.Buffer{}
	tmpl, err := template.New("template").Parse(text)
	if err != nil {
		return buff.String(), err
	}
	err = tmpl.Execute(&buff, data)
	return buff.String(), err
}

// ValidationContext is used for validating templates and pages
type ValidationContext struct {
	FromAddress string
	BaseURL     string
}

func (vc ValidationContext) getFromAddress() string {
	return vc.FromAddress
}

func (vc ValidationContext) getBaseURL() string {
	return vc.BaseURL
}

// ValidateTemplate ensures that the provided text in the page or template
// uses the supported template variables correctly.
func ValidateTemplate(text string) error {
	vc := ValidationContext{
		FromAddress: "foo@bar.com",
		BaseURL:     "http://example.com",
	}
	td := Result{
		BaseRecipient: BaseRecipient{
			Email:     "foo@bar.com",
			FirstName: "Foo",
			LastName:  "Bar",
			Position:  "Test",
		},
		RId: "123456",
	}
	ptx, err := NewPhishingTemplateContext(vc, td.BaseRecipient, td.RId)
	if err != nil {
		return err
	}
	_, err = ExecuteTemplate(text, ptx)
	if err != nil {
		return humanizeTemplateError(err)
	}
	return nil
}

var (
	// templateErrFunctionNotDefined matches the parse-time error text/template
	// produces for e.g. {{URL}}: without the leading dot, "URL" is parsed as
	// a call to an undefined function rather than a field access.
	templateErrFunctionNotDefined = regexp.MustCompile(`function "(\w+)" not defined`)
	// templateErrFieldNotFound matches the execution-time error for a
	// dotted reference to a field that doesn't exist, e.g. {{.Url}}.
	templateErrFieldNotFound = regexp.MustCompile(`can't evaluate field (\w+) in type`)
	// templateErrUnclosedAction matches a template with a missing "}}".
	templateErrUnclosedAction = regexp.MustCompile(`unclosed action`)
)

// availableTemplateFields lists the variables ValidateTemplate's error hints
// point users at when a field name doesn't resolve. Keep in sync with
// PhishingTemplateContext and BaseRecipient.
const availableTemplateFields = ".URL, .TrackingURL, .BaseURL, .Tracker, .From, .RId, .Domain, " +
	".FirstName, .LastName, .Position, .Email"

// humanizeTemplateError rewrites the most common Go text/template syntax
// errors into messages a user without Go template knowledge can act on.
// Errors that don't match a known pattern are returned unchanged.
func humanizeTemplateError(err error) error {
	msg := err.Error()
	switch {
	case templateErrFunctionNotDefined.MatchString(msg):
		field := templateErrFunctionNotDefined.FindStringSubmatch(msg)[1]
		return fmt.Errorf("unknown template variable %q - template fields need a leading dot, e.g. use {{.%s}} instead of {{%s}} (available fields: %s)",
			field, field, field, availableTemplateFields)
	case templateErrFieldNotFound.MatchString(msg):
		field := templateErrFieldNotFound.FindStringSubmatch(msg)[1]
		return fmt.Errorf("unknown template variable %q - check the spelling and capitalization (available fields: %s)",
			field, availableTemplateFields)
	case templateErrUnclosedAction.MatchString(msg):
		return fmt.Errorf("template has an unclosed {{ tag - check for a missing }}")
	}
	return err
}
