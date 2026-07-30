package models

import (
	"gopkg.in/check.v1"
)

// TestPostIMAP verifies that saving an IMAP configuration succeeds. IMAP
// has no primary key (it's a per-user singleton keyed by user_id), which
// previously made PostIMAP's db.Save() fall through to an Update with no
// derivable WHERE clause - gorm's global-update guard correctly refused
// that with "WHERE conditions required" on every save, even the first one.
func (s *ModelsSuite) TestPostIMAP(c *check.C) {
	im := &IMAP{
		UserId:   1,
		Enabled:  true,
		Host:     "127.0.0.1",
		Port:     993,
		Username: "test",
		Password: "test",
		TLS:      true,
	}
	c.Assert(PostIMAP(im, 1), check.Equals, nil)

	got, err := GetIMAP(1)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got), check.Equals, 1)
	c.Assert(got[0].Host, check.Equals, "127.0.0.1")
}

// TestPostIMAPReplacesExisting verifies that saving IMAP settings a second
// time replaces the previous configuration rather than accumulating rows
// or failing.
func (s *ModelsSuite) TestPostIMAPReplacesExisting(c *check.C) {
	im := &IMAP{
		UserId:   1,
		Enabled:  true,
		Host:     "127.0.0.1",
		Port:     993,
		Username: "test",
		Password: "test",
		TLS:      true,
	}
	c.Assert(PostIMAP(im, 1), check.Equals, nil)

	im2 := &IMAP{
		UserId:   1,
		Enabled:  true,
		Host:     "127.0.0.2",
		Port:     993,
		Username: "test2",
		Password: "test2",
		TLS:      true,
	}
	c.Assert(PostIMAP(im2, 1), check.Equals, nil)

	got, err := GetIMAP(1)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(got), check.Equals, 1)
	c.Assert(got[0].Host, check.Equals, "127.0.0.2")
}
