package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	log "github.com/raginx/gophish-ng/logger"
	"gorm.io/gorm"
)

// The Phish Scale is NIST's method for rating how difficult a phishing message
// is for its target audience to detect. It reads two independent assessments
// together: the number of observable detection cues in the message and how
// well the message's premise aligns with the target audience's work context.
// A click rate is only interpretable against this difficulty - a low click
// rate on a "very difficult" phish is a very different signal than the same
// rate on an "easy" one.
//
// Reference: Steves, Greene, Theofanos, "A Phish Scale: Rating Human Phishing
// Message Detection Difficulty" (NIST, 2019/2020). The cue catalogue is Table
// II / Appendix A; the detection-difficulty lookup is Table I.

// Premise alignment categories. An empty value means premise alignment has not
// been assessed yet, so no detection difficulty can be computed.
const (
	PremiseAlignmentHigh   = "high"
	PremiseAlignmentMedium = "medium"
	PremiseAlignmentLow    = "low"
)

// Cue bands. NIST bins the summed cue instance count into three bands, where
// fewer cues means the phish is harder to detect. An empty band means no cues
// have been counted yet.
const (
	CueBandNone = ""     // 0 cues counted - unrated
	CueBandFew  = "few"  // 1-8 cues  (more difficult)
	CueBandSome = "some" // 9-14 cues (moderate)
	CueBandMany = "many" // 15+ cues  (less difficult)
)

// Detection difficulty ratings (Table I). The "some cues + low premise" cell
// is deliberately a dual "moderately to least difficult" rating in the source,
// reflecting NIST's belief that even low premise alignment can have a
// disproportionate effect on increasing detection difficulty.
const (
	DifficultyNone                = ""
	DifficultyVeryDifficult       = "very_difficult"
	DifficultyModeratelyDifficult = "moderately_difficult"
	DifficultyModerateToLeast     = "moderately_to_least_difficult"
	DifficultyLeastDifficult      = "least_difficult"
)

// difficultyLabels maps each difficulty key to its human-readable label as
// phrased in the NIST paper.
var difficultyLabels = map[string]string{
	DifficultyVeryDifficult:       "Very difficult",
	DifficultyModeratelyDifficult: "Moderately difficult",
	DifficultyModerateToLeast:     "Moderately to least difficult",
	DifficultyLeastDifficult:      "Least difficult",
}

// Cue categories (Table II). Keys are stable, deliberately not derived from Go
// identifiers, so renaming a constant never silently invalidates stored data.
const (
	CueCategoryError     = "error"
	CueCategoryTechnical = "technical_indicator"
	CueCategoryVisual    = "visual_presentation"
	CueCategoryLanguage  = "language_and_content"
	CueCategoryTactic    = "common_tactic"
)

// CueDefinition describes one of the observable cues in the NIST catalogue.
// The Key is stored; Category and Name drive presentation.
type CueDefinition struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Name     string `json:"name"`
}

// phishScaleCatalog is the ordered NIST cue catalogue (Table II / Appendix A).
// Order matches the paper so the assessment UI reads the same way. Each cue is
// counted by the number of instances present in the message, and the instance
// counts are summed for the total that feeds the cue band.
var phishScaleCatalog = []CueDefinition{
	{Key: "spelling_grammar", Category: CueCategoryError, Name: "Spelling and grammar irregularities"},
	{Key: "inconsistency", Category: CueCategoryError, Name: "Inconsistency"},

	{Key: "attachment_type", Category: CueCategoryTechnical, Name: "Attachment type"},
	{Key: "sender_display", Category: CueCategoryTechnical, Name: "Sender display name and email address"},
	{Key: "url_hyperlinking", Category: CueCategoryTechnical, Name: "URL hyperlinking"},
	{Key: "domain_spoofing", Category: CueCategoryTechnical, Name: "Domain spoofing"},

	{Key: "no_branding", Category: CueCategoryVisual, Name: "No/minimal branding and logos"},
	{Key: "logo_imitation", Category: CueCategoryVisual, Name: "Logo imitation or out-of-date branding/logos"},
	{Key: "unprofessional_design", Category: CueCategoryVisual, Name: "Unprofessional looking design or formatting"},
	{Key: "security_indicators", Category: CueCategoryVisual, Name: "Security indicators and icons"},

	{Key: "legal_language", Category: CueCategoryLanguage, Name: "Legal language/copyright info/disclaimers"},
	{Key: "distracting_detail", Category: CueCategoryLanguage, Name: "Distracting detail"},
	{Key: "sensitive_request", Category: CueCategoryLanguage, Name: "Requests for sensitive information"},
	{Key: "urgency", Category: CueCategoryLanguage, Name: "Sense of urgency"},
	{Key: "threatening", Category: CueCategoryLanguage, Name: "Threatening language"},
	{Key: "generic_greeting", Category: CueCategoryLanguage, Name: "Generic greeting"},
	{Key: "lack_signer_details", Category: CueCategoryLanguage, Name: "Lack of signer details"},

	{Key: "humanitarian", Category: CueCategoryTactic, Name: "Humanitarian appeals"},
	{Key: "too_good", Category: CueCategoryTactic, Name: "Too good to be true offers"},
	{Key: "youre_special", Category: CueCategoryTactic, Name: "You're special"},
	{Key: "limited_time", Category: CueCategoryTactic, Name: "Limited time offer"},
	{Key: "mimics_process", Category: CueCategoryTactic, Name: "Mimics a work or business process"},
	{Key: "poses_authority", Category: CueCategoryTactic, Name: "Poses as friend, colleague, supervisor, authority figure"},
}

// cueCatalogIndex is a lookup of valid cue keys, built once from the catalogue.
var cueCatalogIndex = func() map[string]bool {
	idx := make(map[string]bool, len(phishScaleCatalog))
	for _, c := range phishScaleCatalog {
		idx[c.Key] = true
	}
	return idx
}()

var (
	// ErrInvalidPremiseAlignment is returned when a premise alignment other
	// than high/medium/low (or empty, meaning unset) is supplied.
	ErrInvalidPremiseAlignment = errors.New("premise alignment must be high, medium, or low")
	// ErrNegativeCueCount is returned when a cue is given a negative count.
	ErrNegativeCueCount = errors.New("phish scale cue counts cannot be negative")
)

// PhishScale is the stored assessment for a single campaign. It is a message-
// and audience-level judgement, not recipient data, so it carries no PII and
// deliberately survives campaign anonymization - that is what keeps a scrubbed
// campaign's report interpretable.
type PhishScale struct {
	Id               int64           `json:"-" gorm:"column:id;primaryKey"`
	CampaignId       int64           `json:"campaign_id" gorm:"column:campaign_id;unique"`
	TeamId           int64           `json:"-" gorm:"column:team_id"`
	PremiseAlignment string          `json:"premise_alignment" gorm:"column:premise_alignment"`
	Notes            string          `json:"notes" gorm:"column:notes"`
	ModifiedDate     time.Time       `json:"modified_date" gorm:"column:modified_date"`
	Cues             []PhishScaleCue `json:"-" gorm:"foreignKey:PhishScaleId"`
}

// TableName pins the table name so gorm doesn't pluralize it.
func (PhishScale) TableName() string { return "phish_scale" }

// PhishScaleCue is one counted cue within an assessment. Only cues with a
// non-zero count are persisted; an absent cue is an implicit count of zero.
type PhishScaleCue struct {
	Id           int64  `json:"-" gorm:"column:id;primaryKey"`
	PhishScaleId int64  `json:"-" gorm:"column:phish_scale_id"`
	CueKey       string `json:"cue_key" gorm:"column:cue_key"`
	Count        int    `json:"count" gorm:"column:count"`
}

// TableName pins the join table name.
func (PhishScaleCue) TableName() string { return "phish_scale_cues" }

// PhishScaleResult is the API-facing view of an assessment: the stored inputs,
// the derived band/difficulty, and the full cue catalogue (so a client always
// renders the same cues the server validates against). Cues maps every
// catalogue key to its current count, defaulting to zero.
type PhishScaleResult struct {
	CampaignId       int64           `json:"campaign_id"`
	Assessed         bool            `json:"assessed"`
	PremiseAlignment string          `json:"premise_alignment"`
	Notes            string          `json:"notes"`
	ModifiedDate     time.Time       `json:"modified_date"`
	Cues             map[string]int  `json:"cues"`
	CueCount         int             `json:"cue_count"`
	CueBand          string          `json:"cue_band"`
	Difficulty       string          `json:"difficulty"`
	DifficultyLabel  string          `json:"difficulty_label"`
	Catalog          []CueDefinition `json:"catalog"`
}

// PhishScaleInput is the client-supplied payload for saving an assessment.
type PhishScaleInput struct {
	PremiseAlignment string         `json:"premise_alignment"`
	Notes            string         `json:"notes"`
	Cues             map[string]int `json:"cues"`
}

// validPremiseAlignment reports whether p is a recognized premise alignment.
func validPremiseAlignment(p string) bool {
	switch p {
	case PremiseAlignmentHigh, PremiseAlignmentMedium, PremiseAlignmentLow:
		return true
	default:
		return false
	}
}

// cueBandFor bins a summed cue instance count into a NIST cue band.
func cueBandFor(count int) string {
	switch {
	case count <= 0:
		return CueBandNone
	case count <= 8:
		return CueBandFew
	case count <= 14:
		return CueBandSome
	default:
		return CueBandMany
	}
}

// difficultyFor implements the NIST Phish Scale detection-difficulty lookup
// (Table I). A missing band or premise alignment yields no rating, because the
// paper is explicit that neither component alone determines difficulty.
func difficultyFor(band, premise string) string {
	if band == CueBandNone || premise == "" {
		return DifficultyNone
	}
	switch band {
	case CueBandFew:
		switch premise {
		case PremiseAlignmentHigh, PremiseAlignmentMedium:
			return DifficultyVeryDifficult
		case PremiseAlignmentLow:
			return DifficultyModeratelyDifficult
		}
	case CueBandSome:
		switch premise {
		case PremiseAlignmentHigh:
			return DifficultyVeryDifficult
		case PremiseAlignmentMedium:
			return DifficultyModeratelyDifficult
		case PremiseAlignmentLow:
			return DifficultyModerateToLeast
		}
	case CueBandMany:
		switch premise {
		case PremiseAlignmentHigh, PremiseAlignmentMedium:
			return DifficultyModeratelyDifficult
		case PremiseAlignmentLow:
			return DifficultyLeastDifficult
		}
	}
	return DifficultyNone
}

// newPhishScaleResult returns an unassessed result seeded with the full cue
// catalogue (every key at zero), ready to be populated.
func newPhishScaleResult(campaignID int64) PhishScaleResult {
	cues := make(map[string]int, len(phishScaleCatalog))
	for _, c := range phishScaleCatalog {
		cues[c.Key] = 0
	}
	return PhishScaleResult{
		CampaignId: campaignID,
		Cues:       cues,
		Catalog:    phishScaleCatalog,
	}
}

// finalize recomputes the derived fields from the current cue counts and
// premise alignment. It is the single place band/difficulty are derived, so a
// read can never disagree with a freshly saved assessment.
func (r *PhishScaleResult) finalize() {
	total := 0
	for _, n := range r.Cues {
		if n > 0 {
			total += n
		}
	}
	r.CueCount = total
	r.CueBand = cueBandFor(total)
	r.Difficulty = difficultyFor(r.CueBand, r.PremiseAlignment)
	r.DifficultyLabel = difficultyLabels[r.Difficulty]
}

// GetPhishScale returns the Phish Scale assessment for the campaign visible to
// teamID. When no assessment has been saved yet it returns an unassessed
// result (Assessed=false) rather than an error, so the UI can render a blank
// form. The campaign itself must exist for the team, or the campaign's lookup
// error is returned.
func GetPhishScale(campaignID, teamID int64) (PhishScaleResult, error) {
	res := newPhishScaleResult(campaignID)

	c := Campaign{}
	if err := db.Where("id = ? AND team_id = ?", campaignID, teamID).First(&c).Error; err != nil {
		log.Error(err)
		return res, err
	}

	ps := PhishScale{}
	err := db.Preload("Cues").Where("campaign_id = ? AND team_id = ?", campaignID, teamID).First(&ps).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return res, nil
		}
		log.Error(err)
		return res, err
	}

	res.Assessed = true
	res.PremiseAlignment = ps.PremiseAlignment
	res.Notes = ps.Notes
	res.ModifiedDate = ps.ModifiedDate
	for _, cue := range ps.Cues {
		// Ignore keys no longer in the catalogue so a catalogue change can't
		// resurrect a stale cue into the total.
		if _, ok := res.Cues[cue.CueKey]; ok {
			res.Cues[cue.CueKey] = cue.Count
		}
	}
	res.finalize()
	return res, nil
}

// SavePhishScale validates and upserts the assessment for the campaign visible
// to teamID, replacing any previously stored cues, and returns the recomputed
// result. Premise alignment may be empty (unset); when present it must be
// high/medium/low. Cue keys must be in the catalogue and counts non-negative.
func SavePhishScale(campaignID, teamID int64, in PhishScaleInput) (PhishScaleResult, error) {
	res := newPhishScaleResult(campaignID)

	premise := strings.ToLower(strings.TrimSpace(in.PremiseAlignment))
	if premise != "" && !validPremiseAlignment(premise) {
		return res, ErrInvalidPremiseAlignment
	}
	for key, count := range in.Cues {
		if !cueCatalogIndex[key] {
			return res, fmt.Errorf("unknown phish scale cue: %s", key)
		}
		if count < 0 {
			return res, ErrNegativeCueCount
		}
	}

	c := Campaign{}
	if err := db.Where("id = ? AND team_id = ?", campaignID, teamID).First(&c).Error; err != nil {
		log.Error(err)
		return res, err
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		ps := PhishScale{}
		e := tx.Where("campaign_id = ? AND team_id = ?", campaignID, teamID).First(&ps).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		ps.CampaignId = campaignID
		ps.TeamId = teamID
		ps.PremiseAlignment = premise
		ps.Notes = in.Notes
		ps.ModifiedDate = time.Now().UTC()
		// Save inserts when Id is zero and updates otherwise.
		if e := tx.Save(&ps).Error; e != nil {
			return e
		}

		// Replace the cue set wholesale so removed cues don't linger.
		if e := tx.Where("phish_scale_id = ?", ps.Id).Delete(&PhishScaleCue{}).Error; e != nil {
			return e
		}
		for key, count := range in.Cues {
			if count <= 0 {
				continue
			}
			cue := PhishScaleCue{PhishScaleId: ps.Id, CueKey: key, Count: count}
			if e := tx.Create(&cue).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		log.Error(err)
		return res, err
	}

	return GetPhishScale(campaignID, teamID)
}

// deletePhishScaleFor removes a campaign's assessment and its cues. It is a
// cleanup helper for campaign deletion; a campaign with no assessment is a
// no-op.
func deletePhishScaleFor(campaignID int64) error {
	ps := PhishScale{}
	err := db.Where("campaign_id = ?", campaignID).First(&ps).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if err := db.Where("phish_scale_id = ?", ps.Id).Delete(&PhishScaleCue{}).Error; err != nil {
		return err
	}
	return db.Delete(&ps).Error
}
