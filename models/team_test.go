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

// TestDifferentTeamsAreIsolatedForAllObjectTypes covers the same isolation
// invariant as TestDifferentTeamsAreIsolated, but for every remaining
// team-scoped object type
func (s *ModelsSuite) TestDifferentTeamsAreIsolatedForAllObjectTypes(c *check.C) {
	admin, err := GetUser(1)
	c.Assert(err, check.Equals, nil)

	otherTeam, err := GetOrCreateTeamByName("Engagement Gamma")
	c.Assert(err, check.Equals, nil)
	outsider := s.newTeamUser(c, "outsider-objects", otherTeam.Id)

	// One object of each type, owned by the admin's team. TeamId is derived
	// from the creating user by the Post* helpers.
	group := Group{Name: "Isolation Group", UserId: admin.Id}
	group.Targets = []Target{
		Target{BaseRecipient: BaseRecipient{Email: "isolation@example.com"}},
	}
	c.Assert(PostGroup(&group), check.Equals, nil)

	template := Template{UserId: admin.Id, Name: "Isolation Template", Text: "Text"}
	c.Assert(PostTemplate(&template), check.Equals, nil)

	page := Page{UserId: admin.Id, Name: "Isolation Page", HTML: "<html>Test</html>"}
	c.Assert(PostPage(&page), check.Equals, nil)

	smtp := SMTP{UserId: admin.Id, Name: "Isolation Profile", Host: "1.1.1.1:25", FromAddress: "foo@example.com"}
	c.Assert(PostSMTP(&smtp), check.Equals, nil)

	tests := []struct {
		name string
		id   int64
		// listIDs returns the IDs the given team can see.
		listIDs func(teamID int64) ([]int64, error)
		// getByID fetches a single object as the given team.
		getByID func(id int64, teamID int64) error
	}{
		{
			name: "groups", id: group.Id,
			listIDs: func(teamID int64) ([]int64, error) {
				gs, err := GetGroups(teamID)
				ids := []int64{}
				for _, g := range gs {
					ids = append(ids, g.Id)
				}
				return ids, err
			},
			getByID: func(id int64, teamID int64) error {
				_, err := GetGroup(id, teamID)
				return err
			},
		},
		{
			name: "templates", id: template.Id,
			listIDs: func(teamID int64) ([]int64, error) {
				ts, err := GetTemplates(teamID)
				ids := []int64{}
				for _, t := range ts {
					ids = append(ids, t.Id)
				}
				return ids, err
			},
			getByID: func(id int64, teamID int64) error {
				_, err := GetTemplate(id, teamID)
				return err
			},
		},
		{
			name: "pages", id: page.Id,
			listIDs: func(teamID int64) ([]int64, error) {
				ps, err := GetPages(teamID)
				ids := []int64{}
				for _, p := range ps {
					ids = append(ids, p.Id)
				}
				return ids, err
			},
			getByID: func(id int64, teamID int64) error {
				_, err := GetPage(id, teamID)
				return err
			},
		},
		{
			name: "sending profiles", id: smtp.Id,
			listIDs: func(teamID int64) ([]int64, error) {
				ss, err := GetSMTPs(teamID)
				ids := []int64{}
				for _, s := range ss {
					ids = append(ids, s.Id)
				}
				return ids, err
			},
			getByID: func(id int64, teamID int64) error {
				_, err := GetSMTP(id, teamID)
				return err
			},
		},
	}

	for _, tc := range tests {
		comment := check.Commentf("object type: %s", tc.name)

		// The owning team still sees it - otherwise the assertions below
		// would pass simply because nothing was created.
		ownIDs, err := tc.listIDs(admin.TeamID)
		c.Assert(err, check.Equals, nil, comment)
		found := false
		for _, id := range ownIDs {
			if id == tc.id {
				found = true
			}
		}
		c.Assert(found, check.Equals, true, comment)
		c.Assert(tc.getByID(tc.id, admin.TeamID), check.Equals, nil, comment)

		// The outsider sees neither the object in their list...
		otherIDs, err := tc.listIDs(outsider.TeamID)
		c.Assert(err, check.Equals, nil, comment)
		for _, id := range otherIDs {
			c.Assert(id, check.Not(check.Equals), tc.id, comment)
		}
		// ...nor by fetching it directly by ID.
		c.Assert(tc.getByID(tc.id, outsider.TeamID), check.Equals, gorm.ErrRecordNotFound, comment)
	}
}

// TestCampaignAccessorsAreTeamScoped covers the campaign entry points beyond
// GetCampaign/GetCampaigns
func (s *ModelsSuite) TestCampaignAccessorsAreTeamScoped(c *check.C) {
	admin, err := GetUser(1)
	c.Assert(err, check.Equals, nil)

	otherTeam, err := GetOrCreateTeamByName("Engagement Delta")
	c.Assert(err, check.Equals, nil)
	outsider := s.newTeamUser(c, "outsider-campaign", otherTeam.Id)

	campaign := s.createCampaignDependencies(c)
	c.Assert(PostCampaign(&campaign, admin.Id, admin.TeamID), check.Equals, nil)

	// Accessors that report a miss as a record-not-found error.
	notFound := map[string]func(id int64, teamID int64) error{
		"GetCampaignResults": func(id int64, teamID int64) error {
			_, err := GetCampaignResults(id, teamID)
			return err
		},
		"GetCampaignMailContext": func(id int64, teamID int64) error {
			_, err := GetCampaignMailContext(id, teamID)
			return err
		},
		"GetCampaignPhishContext": func(id int64, teamID int64) error {
			_, err := GetCampaignPhishContext(id, teamID)
			return err
		},
	}
	for name, accessor := range notFound {
		comment := check.Commentf("accessor: %s", name)
		c.Assert(accessor(campaign.Id, admin.TeamID), check.Equals, nil, comment)
		c.Assert(accessor(campaign.Id, outsider.TeamID), check.Equals, gorm.ErrRecordNotFound, comment)
	}

	// GetCampaignSummary scans into a struct rather than using First(), so a
	// cross-team lookup returns an empty summary instead of an error
	summary, err := GetCampaignSummary(campaign.Id, admin.TeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(summary.Id, check.Equals, campaign.Id)

	summary, err = GetCampaignSummary(campaign.Id, outsider.TeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(summary.Id, check.Equals, int64(0))
	c.Assert(summary.Name, check.Equals, "")

	summaries, err := GetCampaignSummaries(outsider.TeamID)
	c.Assert(err, check.Equals, nil)
	for _, cs := range summaries.Campaigns {
		c.Assert(cs.Id, check.Not(check.Equals), campaign.Id)
	}
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
