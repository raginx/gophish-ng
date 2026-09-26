package models

import (
	"errors"
	"sort"
	"strings"
	"time"

	log "github.com/raginx/gophish-ng/logger"
	"gorm.io/gorm"
)

// Taggable type identifiers stored in taggables.taggable_type. These are
// stable string keys, deliberately not derived from Go type names, so
// renaming a struct never silently detaches existing tags.
const (
	TaggableTemplate = "template"
	TaggablePage     = "page"
	TaggableSMTP     = "smtp"
	TaggableCampaign = "campaign"
	TaggableGroup    = "group"
)

// Tag is a team-scoped label that can be attached to any taggable object
// (templates, pages, sending profiles, campaigns, groups) via the
// taggables join table. Tags are shared across object types within a team:
// a "de" tag is a single row that can label both a template and a page.
type Tag struct {
	Id           int64     `json:"id" gorm:"column:id;primaryKey"`
	TeamId       int64     `json:"-" gorm:"column:team_id"`
	Name         string    `json:"name"`
	Color        string    `json:"color,omitempty"`
	ModifiedDate time.Time `json:"modified_date"`
}

// taggable is the polymorphic join row linking a Tag to an object. It's an
// internal type - callers work with tag names via the helpers below.
type taggable struct {
	TagId        int64  `gorm:"column:tag_id"`
	TaggableType string `gorm:"column:taggable_type"`
	TaggableId   int64  `gorm:"column:taggable_id"`
}

// TableName pins the join table name so gorm doesn't pluralize it oddly.
func (taggable) TableName() string { return "taggables" }

// ErrTagNameNotSpecified is thrown when a tag name is blank.
var ErrTagNameNotSpecified = errors.New("tag name not specified")

// normalizeTagNames trims, drops empties, de-duplicates (case-insensitive)
// and sorts a set of tag names so storage and comparison are consistent.
func normalizeTagNames(names []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

// GetTags returns every tag defined for the team, ordered by name.
func GetTags(teamID int64) ([]Tag, error) {
	tags := []Tag{}
	err := db.Where("team_id=?", teamID).Order("name").Find(&tags).Error
	if err != nil {
		log.Error(err)
	}
	return tags, err
}

// GetTag returns a single tag scoped to the team.
func GetTag(id int64, teamID int64) (Tag, error) {
	tag := Tag{}
	err := db.Where("team_id=? AND id=?", teamID, id).First(&tag).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error(err)
	}
	return tag, err
}

// PostTag creates a new tag for the team. Tags are usually created
// implicitly by tagging an object, but the explicit endpoint lets the UI
// pre-create a tag (e.g. to assign a color).
func PostTag(t *Tag) error {
	if strings.TrimSpace(t.Name) == "" {
		return ErrTagNameNotSpecified
	}
	t.Name = strings.TrimSpace(t.Name)
	t.ModifiedDate = time.Now().UTC()
	err := db.Save(t).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// PutTag renames and/or recolors an existing tag. The unique (team_id,
// name) index enforces name collisions, surfaced to the caller as
// gorm.ErrDuplicatedKey.
func PutTag(t *Tag) error {
	if strings.TrimSpace(t.Name) == "" {
		return ErrTagNameNotSpecified
	}
	t.Name = strings.TrimSpace(t.Name)
	t.ModifiedDate = time.Now().UTC()
	err := db.Where("id=? AND team_id=?", t.Id, t.TeamId).Save(t).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// DeleteTag removes a tag and all of its object links within the team. The
// linked objects themselves are left untouched.
func DeleteTag(id int64, teamID int64) error {
	tag, err := GetTag(id, teamID)
	if err != nil {
		return err
	}
	if err := db.Where("tag_id=?", tag.Id).Delete(&taggable{}).Error; err != nil {
		log.Error(err)
		return err
	}
	err = db.Where("team_id=? AND id=?", teamID, tag.Id).Delete(&Tag{}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// getOrCreateTag resolves a tag by (team, name), creating it if it doesn't
// exist yet. Matching is case-insensitive so "DE" and "de" don't create
// two rows.
func getOrCreateTag(name string, teamID int64) (Tag, error) {
	tag := Tag{}
	err := db.Where("team_id=? AND LOWER(name)=LOWER(?)", teamID, name).First(&tag).Error
	if err == nil {
		return tag, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error(err)
		return tag, err
	}
	tag = Tag{TeamId: teamID, Name: name, ModifiedDate: time.Now().UTC()}
	if err := db.Save(&tag).Error; err != nil {
		// A concurrent request may have created the same tag between our
		// SELECT and INSERT; fall back to reading the existing row.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if err2 := db.Where("team_id=? AND LOWER(name)=LOWER(?)", teamID, name).First(&tag).Error; err2 == nil {
				return tag, nil
			}
		}
		log.Error(err)
		return tag, err
	}
	return tag, nil
}

// getTagsFor returns the tag names attached to a single object, sorted.
func getTagsFor(taggableType string, id int64, teamID int64) ([]string, error) {
	names := []string{}
	err := db.Model(&Tag{}).
		Joins("JOIN taggables ON taggables.tag_id = tags.id").
		Where("taggables.taggable_type=? AND taggables.taggable_id=? AND tags.team_id=?", taggableType, id, teamID).
		Order("tags.name").
		Pluck("tags.name", &names).Error
	if err != nil {
		log.Error(err)
	}
	if names == nil {
		names = []string{}
	}
	return names, err
}

// setTagsFor replaces the full set of tags on an object with the given
// names (resolve-or-create per name), removing any links no longer present.
// Passing an empty/nil slice clears every tag from the object. The tag
// rows themselves are shared team-wide and are never deleted here.
func setTagsFor(taggableType string, id int64, teamID int64, names []string) error {
	names = normalizeTagNames(names)
	want := map[int64]bool{}
	for _, n := range names {
		tag, err := getOrCreateTag(n, teamID)
		if err != nil {
			return err
		}
		want[tag.Id] = true
	}
	// Load the object's current links, scoped to the team so we never
	// touch another team's tag that happens to share an id.
	current := []taggable{}
	err := db.Model(&taggable{}).
		Joins("JOIN tags ON tags.id = taggables.tag_id").
		Where("taggables.taggable_type=? AND taggables.taggable_id=? AND tags.team_id=?", taggableType, id, teamID).
		Select("taggables.*").
		Find(&current).Error
	if err != nil {
		log.Error(err)
		return err
	}
	have := map[int64]bool{}
	for _, t := range current {
		have[t.TagId] = true
	}
	for tagID := range want {
		if have[tagID] {
			continue
		}
		link := taggable{TagId: tagID, TaggableType: taggableType, TaggableId: id}
		if err := db.Create(&link).Error; err != nil {
			// Ignore a duplicate inserted by a concurrent request.
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				continue
			}
			log.Error(err)
			return err
		}
	}
	for tagID := range have {
		if want[tagID] {
			continue
		}
		if err := db.Where("tag_id=? AND taggable_type=? AND taggable_id=?", tagID, taggableType, id).
			Delete(&taggable{}).Error; err != nil {
			log.Error(err)
			return err
		}
	}
	return nil
}

// deleteTagsFor removes every tag link for an object, called when the
// object itself is deleted. The tags are left intact since they're shared
// team-wide.
func deleteTagsFor(taggableType string, id int64) error {
	err := db.Where("taggable_type=? AND taggable_id=?", taggableType, id).Delete(&taggable{}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// TaggedIDs returns the ids of objects of the given type in the team that
// carry at least one of the named tags (OR semantics). An empty name set
// returns no ids.
func TaggedIDs(taggableType string, teamID int64, names []string) ([]int64, error) {
	names = normalizeTagNames(names)
	ids := []int64{}
	if len(names) == 0 {
		return ids, nil
	}
	lower := make([]string, len(names))
	for i, n := range names {
		lower[i] = strings.ToLower(n)
	}
	err := db.Model(&taggable{}).
		Joins("JOIN tags ON tags.id = taggables.tag_id").
		Where("taggables.taggable_type=? AND tags.team_id=? AND LOWER(tags.name) IN ?", taggableType, teamID, lower).
		Distinct().
		Pluck("taggables.taggable_id", &ids).Error
	if err != nil {
		log.Error(err)
	}
	return ids, err
}
