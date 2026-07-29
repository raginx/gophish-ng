package models

import (
	"errors"

	"gopkg.in/check.v1"
	"gorm.io/gorm"
)

// TestPostTemplateDuplicateName verifies that the database itself rejects
// a second template with the same (user_id, name)
func (s *ModelsSuite) TestPostTemplateDuplicateName(c *check.C) {
	t1 := Template{Name: "Duplicate Template", Text: "Text"}
	c.Assert(PostTemplate(&t1), check.Equals, nil)

	t2 := Template{Name: "Duplicate Template", Text: "Text"}
	err := PostTemplate(&t2)
	c.Assert(errors.Is(err, gorm.ErrDuplicatedKey), check.Equals, true)
}
