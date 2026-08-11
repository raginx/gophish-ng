package models

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ErrTeamNameNotSpecified is thrown when a team is created without a name.
var ErrTeamNameNotSpecified = errors.New("Team name not specified")

// Team groups users together so that the objects they create (campaigns,
// templates, pages, groups, sending profiles, IMAP configs) are visible to
// every member of the team, rather than being strictly siloed to the
// individual creator. See the design note in rbac.go.
type Team struct {
	Id           int64     `json:"id"`
	Name         string    `json:"name" gorm:"column:name;not null;unique"`
	ModifiedDate time.Time `json:"modified_date"`
}

// GetTeams returns every team registered in Gophish.
func GetTeams() ([]Team, error) {
	ts := []Team{}
	err := db.Find(&ts).Error
	return ts, err
}

// GetTeam returns the team that the given id corresponds to. If no team is
// found, an error is thrown.
func GetTeam(id int64) (Team, error) {
	t := Team{}
	err := db.Where("id=?", id).First(&t).Error
	return t, err
}

// GetTeamByName returns the team with the given name. If no team is found,
// an error is thrown.
func GetTeamByName(name string) (Team, error) {
	t := Team{}
	err := db.Where("name=?", name).First(&t).Error
	return t, err
}

// PostTeam creates a new team.
func PostTeam(t *Team) error {
	if t.Name == "" {
		return ErrTeamNameNotSpecified
	}
	t.ModifiedDate = time.Now().UTC()
	return db.Save(t).Error
}

// setTeamIdFromUser fills in *teamID from the given user's own team if it's
// not already set. Every production caller (the controllers) sets TeamId
// explicitly already (it's cached in the request context, avoiding an
// extra lookup here) - this only matters for callers that construct
// objects directly and only set UserId, the same way AuthType/Folder
// elsewhere in this package default when left unspecified.
func setTeamIdFromUser(teamID *int64, userID int64) error {
	if *teamID != 0 {
		return nil
	}
	u, err := GetUser(userID)
	if err != nil {
		return err
	}
	*teamID = u.TeamID
	return nil
}

// GetOrCreateTeamByName returns the team with the given name, creating it
// if it doesn't already exist. There's no dedicated team management UI -
// admins assign a team to a user by name directly on that user's account,
// so a not-yet-seen name is how a new team gets created.
func GetOrCreateTeamByName(name string) (Team, error) {
	t, err := GetTeamByName(name)
	if err == gorm.ErrRecordNotFound {
		t = Team{Name: name}
		err = PostTeam(&t)
	}
	return t, err
}
