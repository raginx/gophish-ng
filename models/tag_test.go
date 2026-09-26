package models

import (
	"gopkg.in/check.v1"
)

// user 1 belongs to the seeded "Default Team" (id 1), so every object
// created with UserId 1 in these tests is scoped to team 1.
const testTeamID = int64(1)

// TestTemplateTagsRoundTrip verifies tags set on a template are persisted,
// returned sorted on read, and fully replaced (not merged) on update.
func (s *ModelsSuite) TestTemplateTagsRoundTrip(c *check.C) {
	t := Template{UserId: 1, Name: "Tagged Template", Text: "Text", Tags: []string{"invoice", "de"}}
	c.Assert(PostTemplate(&t), check.Equals, nil)

	got, err := GetTemplate(t.Id, testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Tags, check.DeepEquals, []string{"de", "invoice"})

	// PutTemplate replaces the whole set: "invoice" drops off the object.
	t.Tags = []string{"de", "urgent"}
	c.Assert(PutTemplate(&t), check.Equals, nil)
	got, err = GetTemplate(t.Id, testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Tags, check.DeepEquals, []string{"de", "urgent"})
}

// TestTagsAreCaseInsensitiveAndDeduplicated verifies that tag names differing
// only in case or whitespace resolve to a single tag row per team.
func (s *ModelsSuite) TestTagsAreCaseInsensitiveAndDeduplicated(c *check.C) {
	t := Template{UserId: 1, Name: "Dedup Template", Text: "Text", Tags: []string{"DE", " de ", "de"}}
	c.Assert(PostTemplate(&t), check.Equals, nil)

	got, err := GetTemplate(t.Id, testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Tags, check.HasLen, 1)

	tags, err := GetTags(testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(tags, check.HasLen, 1)
}

// TestTagsSharedAcrossObjectTypes verifies a single tag row is reused when
// the same name is applied to different object types within a team.
func (s *ModelsSuite) TestTagsSharedAcrossObjectTypes(c *check.C) {
	t := Template{UserId: 1, Name: "Shared Tag Template", Text: "Text", Tags: []string{"phishing"}}
	c.Assert(PostTemplate(&t), check.Equals, nil)
	p := Page{UserId: 1, Name: "Shared Tag Page", HTML: "<html></html>", Tags: []string{"phishing"}}
	c.Assert(PostPage(&p), check.Equals, nil)

	tags, err := GetTags(testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(tags, check.HasLen, 1)
	c.Assert(tags[0].Name, check.Equals, "phishing")
}

// TestTaggedIDsFiltering verifies TaggedIDs returns only objects carrying at
// least one of the requested tags (OR semantics), scoped to object type.
func (s *ModelsSuite) TestTaggedIDsFiltering(c *check.C) {
	de := Template{UserId: 1, Name: "DE Template", Text: "Text", Tags: []string{"de"}}
	c.Assert(PostTemplate(&de), check.Equals, nil)
	en := Template{UserId: 1, Name: "EN Template", Text: "Text", Tags: []string{"en"}}
	c.Assert(PostTemplate(&en), check.Equals, nil)
	// A page tagged "de" must not leak into a template-scoped query.
	p := Page{UserId: 1, Name: "DE Page", HTML: "<html></html>", Tags: []string{"de"}}
	c.Assert(PostPage(&p), check.Equals, nil)

	ids, err := TaggedIDs(TaggableTemplate, testTeamID, []string{"de"})
	c.Assert(err, check.Equals, nil)
	c.Assert(ids, check.DeepEquals, []int64{de.Id})

	// Case-insensitive match.
	ids, err = TaggedIDs(TaggableTemplate, testTeamID, []string{"DE", "EN"})
	c.Assert(err, check.Equals, nil)
	c.Assert(ids, check.HasLen, 2)
}

// TestDeleteTemplateRemovesTagLinks verifies deleting an object removes its
// tag links but leaves the shared tag rows intact.
func (s *ModelsSuite) TestDeleteTemplateRemovesTagLinks(c *check.C) {
	t := Template{UserId: 1, Name: "Doomed Template", Text: "Text", Tags: []string{"keep"}}
	c.Assert(PostTemplate(&t), check.Equals, nil)

	c.Assert(DeleteTemplate(t.Id, testTeamID), check.Equals, nil)

	var linkCount int64
	c.Assert(db.Model(&taggable{}).
		Where("taggable_type=? AND taggable_id=?", TaggableTemplate, t.Id).
		Count(&linkCount).Error, check.Equals, nil)
	c.Assert(linkCount, check.Equals, int64(0))

	// The tag itself survives for reuse by other objects.
	tags, err := GetTags(testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(tags, check.HasLen, 1)
}

// TestDeleteTagRemovesLinks verifies deleting a tag also removes every link
// to it, so objects no longer report the deleted tag.
func (s *ModelsSuite) TestDeleteTagRemovesLinks(c *check.C) {
	t := Template{UserId: 1, Name: "Retagged Template", Text: "Text", Tags: []string{"temporary"}}
	c.Assert(PostTemplate(&t), check.Equals, nil)

	tags, err := GetTags(testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(tags, check.HasLen, 1)

	c.Assert(DeleteTag(tags[0].Id, testTeamID), check.Equals, nil)

	got, err := GetTemplate(t.Id, testTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Tags, check.HasLen, 0)
}
