package models

import (
	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestCueBandFor(c *check.C) {
	// Boundaries of the NIST cue bands: Few 1-8, Some 9-14, Many 15+.
	c.Assert(cueBandFor(0), check.Equals, CueBandNone)
	c.Assert(cueBandFor(1), check.Equals, CueBandFew)
	c.Assert(cueBandFor(8), check.Equals, CueBandFew)
	c.Assert(cueBandFor(9), check.Equals, CueBandSome)
	c.Assert(cueBandFor(14), check.Equals, CueBandSome)
	c.Assert(cueBandFor(15), check.Equals, CueBandMany)
	c.Assert(cueBandFor(99), check.Equals, CueBandMany)
	// A negative count is defensively treated as unrated, never a band.
	c.Assert(cueBandFor(-3), check.Equals, CueBandNone)
}

func (s *ModelsSuite) TestDifficultyFor(c *check.C) {
	// The full NIST detection-difficulty lookup (Table I).
	cases := []struct {
		band, premise, want string
	}{
		{CueBandFew, PremiseAlignmentHigh, DifficultyVeryDifficult},
		{CueBandFew, PremiseAlignmentMedium, DifficultyVeryDifficult},
		{CueBandFew, PremiseAlignmentLow, DifficultyModeratelyDifficult},
		{CueBandSome, PremiseAlignmentHigh, DifficultyVeryDifficult},
		{CueBandSome, PremiseAlignmentMedium, DifficultyModeratelyDifficult},
		{CueBandSome, PremiseAlignmentLow, DifficultyModerateToLeast},
		{CueBandMany, PremiseAlignmentHigh, DifficultyModeratelyDifficult},
		{CueBandMany, PremiseAlignmentMedium, DifficultyModeratelyDifficult},
		{CueBandMany, PremiseAlignmentLow, DifficultyLeastDifficult},
	}
	for _, tc := range cases {
		c.Assert(difficultyFor(tc.band, tc.premise), check.Equals, tc.want,
			check.Commentf("band=%s premise=%s", tc.band, tc.premise))
	}

	// Neither component alone can produce a rating.
	c.Assert(difficultyFor(CueBandNone, PremiseAlignmentHigh), check.Equals, DifficultyNone)
	c.Assert(difficultyFor(CueBandFew, ""), check.Equals, DifficultyNone)
	c.Assert(difficultyFor(CueBandNone, ""), check.Equals, DifficultyNone)
}

func (s *ModelsSuite) TestPhishScaleCatalog(c *check.C) {
	// The catalogue is the 23-cue NIST Table II, with unique keys and only
	// known categories.
	c.Assert(len(phishScaleCatalog), check.Equals, 23)
	valid := map[string]bool{
		CueCategoryError: true, CueCategoryTechnical: true,
		CueCategoryVisual: true, CueCategoryLanguage: true, CueCategoryTactic: true,
	}
	seen := map[string]bool{}
	for _, cue := range phishScaleCatalog {
		c.Assert(cue.Key, check.Not(check.Equals), "")
		c.Assert(cue.Name, check.Not(check.Equals), "")
		c.Assert(cue.Hint, check.Not(check.Equals), "")
		c.Assert(valid[cue.Category], check.Equals, true, check.Commentf("category=%s", cue.Category))
		c.Assert(seen[cue.Key], check.Equals, false, check.Commentf("duplicate key=%s", cue.Key))
		seen[cue.Key] = true
		c.Assert(cueCatalogIndex[cue.Key], check.Equals, true)
	}
}

func (s *ModelsSuite) TestPhishScaleResultFinalize(c *check.C) {
	r := newPhishScaleResult(1)
	// Seeded unassessed: every cue zero, no band or difficulty.
	c.Assert(len(r.Cues), check.Equals, 23)
	r.finalize()
	c.Assert(r.CueCount, check.Equals, 0)
	c.Assert(r.CueBand, check.Equals, CueBandNone)
	c.Assert(r.Difficulty, check.Equals, DifficultyNone)

	// 6 instances (Few) with a high premise -> Very difficult.
	r.Cues["url_hyperlinking"] = 4
	r.Cues["domain_spoofing"] = 2
	r.PremiseAlignment = PremiseAlignmentHigh
	r.finalize()
	c.Assert(r.CueCount, check.Equals, 6)
	c.Assert(r.CueBand, check.Equals, CueBandFew)
	c.Assert(r.Difficulty, check.Equals, DifficultyVeryDifficult)
	c.Assert(r.DifficultyLabel, check.Equals, "Very difficult")
}

func (s *ModelsSuite) TestSaveAndGetPhishScale(c *check.C) {
	campaign := s.createCampaign(c)

	// An unassessed campaign reads back a blank, seeded result.
	blank, err := GetPhishScale(campaign.Id, campaign.TeamId)
	c.Assert(err, check.Equals, nil)
	c.Assert(blank.Assessed, check.Equals, false)
	c.Assert(blank.CampaignId, check.Equals, campaign.Id)
	c.Assert(len(blank.Catalog), check.Equals, 23)
	c.Assert(blank.Difficulty, check.Equals, DifficultyNone)

	// Save an assessment: 10 cue instances (Some) with a low premise ->
	// "moderately to least difficult".
	saved, err := SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{
		PremiseAlignment: "Low", // mixed case is normalized
		Notes:            "finance OU, invoice pretext",
		Cues: map[string]int{
			"url_hyperlinking": 6,
			"urgency":          3,
			"generic_greeting": 1,
			"humanitarian":     0, // zero is not persisted
		},
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(saved.Assessed, check.Equals, true)
	c.Assert(saved.PremiseAlignment, check.Equals, PremiseAlignmentLow)
	c.Assert(saved.CueCount, check.Equals, 10)
	c.Assert(saved.CueBand, check.Equals, CueBandSome)
	c.Assert(saved.Difficulty, check.Equals, DifficultyModerateToLeast)
	c.Assert(saved.Cues["url_hyperlinking"], check.Equals, 6)
	c.Assert(saved.Cues["humanitarian"], check.Equals, 0)

	// Reading back matches the saved assessment.
	got, err := GetPhishScale(campaign.Id, campaign.TeamId)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Assessed, check.Equals, true)
	c.Assert(got.CueCount, check.Equals, 10)
	c.Assert(got.Difficulty, check.Equals, DifficultyModerateToLeast)
	c.Assert(got.Notes, check.Equals, "finance OU, invoice pretext")

	// Re-saving replaces the cue set wholesale (the old urgency/greeting cues
	// are gone) and recomputes difficulty: 2 cues (Few) + high -> Very difficult.
	updated, err := SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{
		PremiseAlignment: PremiseAlignmentHigh,
		Cues:             map[string]int{"sender_display": 1, "domain_spoofing": 1},
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(updated.CueCount, check.Equals, 2)
	c.Assert(updated.CueBand, check.Equals, CueBandFew)
	c.Assert(updated.Difficulty, check.Equals, DifficultyVeryDifficult)
	c.Assert(updated.Cues["url_hyperlinking"], check.Equals, 0)
	c.Assert(updated.Cues["sender_display"], check.Equals, 1)
}

func (s *ModelsSuite) TestSavePhishScaleValidation(c *check.C) {
	campaign := s.createCampaign(c)

	_, err := SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{PremiseAlignment: "severe"})
	c.Assert(err, check.Equals, ErrInvalidPremiseAlignment)

	_, err = SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{
		Cues: map[string]int{"not_a_real_cue": 1},
	})
	c.Assert(err, check.NotNil)

	_, err = SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{
		Cues: map[string]int{"urgency": -1},
	})
	c.Assert(err, check.Equals, ErrNegativeCueCount)

	// A rejected save leaves nothing persisted.
	got, err := GetPhishScale(campaign.Id, campaign.TeamId)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Assessed, check.Equals, false)
}

func (s *ModelsSuite) TestDeleteCampaignRemovesPhishScale(c *check.C) {
	campaign := s.createCampaign(c)
	_, err := SavePhishScale(campaign.Id, campaign.TeamId, PhishScaleInput{
		PremiseAlignment: PremiseAlignmentHigh,
		Cues:             map[string]int{"urgency": 2},
	})
	c.Assert(err, check.Equals, nil)

	c.Assert(DeleteCampaign(campaign.Id), check.Equals, nil)

	// The assessment and its cues are gone with the campaign.
	var scales, cues int64
	db.Model(&PhishScale{}).Where("campaign_id = ?", campaign.Id).Count(&scales)
	db.Model(&PhishScaleCue{}).Count(&cues)
	c.Assert(scales, check.Equals, int64(0))
	c.Assert(cues, check.Equals, int64(0))
}
