package models

import (
	"fmt"

	check "gopkg.in/check.v1"
)

type mockTemplateContext struct {
	URL         string
	FromAddress string
}

func (m mockTemplateContext) getFromAddress() string {
	return m.FromAddress
}

func (m mockTemplateContext) getBaseURL() string {
	return m.URL
}

func (s *ModelsSuite) TestNewTemplateContext(c *check.C) {
	r := Result{
		BaseRecipient: BaseRecipient{
			FirstName: "Foo",
			LastName:  "Bar",
			Email:     "foo@bar.com",
		},
		RId: "1234567",
	}
	ctx := mockTemplateContext{
		URL:         "http://example.com",
		FromAddress: "From Address <from@example.com>",
	}
	expected := PhishingTemplateContext{
		URL:           fmt.Sprintf("%s?rid=%s", ctx.URL, r.RId),
		BaseURL:       ctx.URL,
		BaseRecipient: r.BaseRecipient,
		TrackingURL:   fmt.Sprintf("%s/track?rid=%s", ctx.URL, r.RId),
		From:          "From Address",
		RId:           r.RId,
		Domain:        "bar.com",
	}
	expected.Tracker = "<img alt='' style='display: none' src='" + expected.TrackingURL + "'/>"
	got, err := NewPhishingTemplateContext(ctx, r.BaseRecipient, r.RId)
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.DeepEquals, expected)
}

func (s *ModelsSuite) TestValidateTemplateMissingDot(c *check.C) {
	// {{URL}} without the leading dot parses as a call to an undefined
	// function named "URL" - the error should point users at {{.URL}}.
	err := ValidateTemplate("Click here: {{URL}}")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, `.*\{\{\.URL\}\}.*`)
	c.Assert(err.Error(), check.Matches, `.*\{\{URL\}\}.*`)
}

func (s *ModelsSuite) TestValidateTemplateUnknownField(c *check.C) {
	err := ValidateTemplate("Click here: {{.Url}}")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, `.*"Url".*`)
	c.Assert(err.Error(), check.Matches, `.*\.URL.*`)
}

func (s *ModelsSuite) TestValidateTemplateUnclosedAction(c *check.C) {
	err := ValidateTemplate("Click here: {{.URL")
	c.Assert(err, check.NotNil)
	c.Assert(err.Error(), check.Matches, `.*unclosed.*\{\{.*`)
}

func (s *ModelsSuite) TestValidateTemplateValid(c *check.C) {
	err := ValidateTemplate("Click here: {{.URL}}")
	c.Assert(err, check.IsNil)
}

func (s *ModelsSuite) TestNewTemplateContextDomainNoAtSign(c *check.C) {
	r := Result{
		BaseRecipient: BaseRecipient{
			FirstName: "Foo",
			LastName:  "Bar",
			Email:     "not-an-email",
		},
		RId: "1234567",
	}
	ctx := mockTemplateContext{
		URL:         "http://example.com",
		FromAddress: "From Address <from@example.com>",
	}
	got, err := NewPhishingTemplateContext(ctx, r.BaseRecipient, r.RId)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Domain, check.Equals, "")
}
