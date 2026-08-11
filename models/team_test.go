package models

import (
	check "gopkg.in/check.v1"
	"gorm.io/gorm"
)

// newTeamUser creates a user with the given role on the given team, for
// tests that need more than one user to exercise team-scoped visibility.
func (s *ModelsSuite) newTeamUser(c *check.C, username string, teamID int64) User {
	role, err := GetRoleBySlug(RoleUser)
	c.Assert(err, check.Equals, nil)
	u := User{
		Username: username,
		Hash:     "hash",
		ApiKey:   username + "-key",
		Role:     role,
		RoleID:   role.ID,
		TeamID:   teamID,
	}
	c.Assert(PutUser(&u), check.Equals, nil)
	return u
}

func (s *ModelsSuite) TestGetOrCreateTeamByNameCreatesOnce(c *check.C) {
	t1, err := GetOrCreateTeamByName("Engagement Alpha")
	c.Assert(err, check.Equals, nil)
	c.Assert(t1.Name, check.Equals, "Engagement Alpha")

	t2, err := GetOrCreateTeamByName("Engagement Alpha")
	c.Assert(err, check.Equals, nil)
	c.Assert(t2.Id, check.Equals, t1.Id)
}

// TestTeamMembersShareCampaigns is the core behavior this feature adds:
// two different users on the same team see and can act on each other's
// campaigns (and, by the same query pattern, templates/pages/groups/smtp).
func (s *ModelsSuite) TestTeamMembersShareCampaigns(c *check.C) {
	admin, err := GetUser(1)
	c.Assert(err, check.Equals, nil)

	teammate := s.newTeamUser(c, "teammate", admin.TeamID)

	campaign := s.createCampaignDependencies(c)
	c.Assert(PostCampaign(&campaign, admin.Id, admin.TeamID), check.Equals, nil)

	// The teammate didn't create this campaign, but shares its creator's
	// team, so it must show up in their team-scoped list...
	campaigns, err := GetCampaigns(teammate.TeamID)
	c.Assert(err, check.Equals, nil)
	found := false
	for _, camp := range campaigns {
		if camp.Id == campaign.Id {
			found = true
		}
	}
	c.Assert(found, check.Equals, true)

	// ...and be individually fetchable by ID.
	got, err := GetCampaign(campaign.Id, teammate.TeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Id, check.Equals, campaign.Id)
}

// TestDifferentTeamsAreIsolated verifies that moving to team-based sharing
// didn't drop isolation entirely - two users on *different* teams still
// can't see each other's campaigns.
func (s *ModelsSuite) TestDifferentTeamsAreIsolated(c *check.C) {
	admin, err := GetUser(1)
	c.Assert(err, check.Equals, nil)

	otherTeam, err := GetOrCreateTeamByName("Engagement Beta")
	c.Assert(err, check.Equals, nil)
	outsider := s.newTeamUser(c, "outsider", otherTeam.Id)

	campaign := s.createCampaignDependencies(c)
	c.Assert(PostCampaign(&campaign, admin.Id, admin.TeamID), check.Equals, nil)

	campaigns, err := GetCampaigns(outsider.TeamID)
	c.Assert(err, check.Equals, nil)
	for _, camp := range campaigns {
		c.Assert(camp.Id, check.Not(check.Equals), campaign.Id)
	}

	_, err = GetCampaign(campaign.Id, outsider.TeamID)
	c.Assert(err, check.Equals, gorm.ErrRecordNotFound)
}
