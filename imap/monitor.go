package imap

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jordan-wright/email"

	"github.com/gophish/gophish/models"
)

// Pattern for GoPhish emails e.g ?rid=AbC1234
// We include the optional quoted-printable 3D at the front, just in case decoding fails. e.g ?rid=3DAbC1234
// We also include alternative URL encoded representations of '=' and '?' to handle Microsoft ATP URLs e.g %3Frid%3DAbC1234
var goPhishRegex = regexp.MustCompile(`((\?|%3F)rid(=|%3D)(3D)?([A-Za-z0-9]{7}))`)

// backoffMax caps how long the monitor will wait between login attempts
const backoffMax = 30 * time.Minute

// backoffDuration returns how long to wait before the next login attempt
func backoffDuration(consecutiveErrors uint32) time.Duration {
	if consecutiveErrors == 0 {
		return 0
	}
	shift := consecutiveErrors - 1
	if shift > 10 { // avoid overflowing the shift; backoffMax caps it anyway
		shift = 10
	}
	d := time.Second * time.Duration(1<<shift)
	if d > backoffMax {
		d = backoffMax
	}
	return d
}

// Monitor is a worker that monitors IMAP servers for reported campaign emails
type Monitor struct {
	cancel func()
}

// Monitor.start() checks for campaign emails
// As each account can have its own polling frequency set we need to run one Go routine for
// each, as well as keeping an eye on newly created user accounts.
func (im *Monitor) start(ctx context.Context) {
	usermap := make(map[int64]int) // Keep track of running go routines, one per user. We assume incrementing non-repeating UIDs (for the case where users are deleted and re-added).

	for {
		select {
		case <-ctx.Done():
			return
		default:
			dbusers, err := models.GetUsers() //Slice of all user ids. Each user gets their own IMAP monitor routine.
			if err != nil {
				log.Error(err)
				break
			}
			for _, dbuser := range dbusers {
				if _, ok := usermap[dbuser.Id]; !ok { // If we don't currently have a running Go routine for this user, start one.
					log.Info("Starting new IMAP monitor for user ", dbuser.Username)
					usermap[dbuser.Id] = 1
					go monitor(dbuser.Id, ctx)
				}
			}
			time.Sleep(10 * time.Second) // Every ten seconds we check if a new user has been created
		}
	}
}

// monitor will continuously login to the IMAP settings associated to the supplied user id (if the user account has IMAP settings, and they're enabled.)
// It also verifies the user account exists, and returns if not (for the case of a user being deleted).
//
// Consecutive login failures extend the sleep between attempts
func monitor(uid int64, ctx context.Context) {
	var consecutiveErrors uint32
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// 1. Check if user exists, if not, return.
			_, err := models.GetUser(uid)
			if err != nil { // Not sure if there's a better way to determine user existence via id.
				log.Info("User ", uid, " seems to have been deleted. Stopping IMAP monitor for this user.")
				return
			}
			// 2. Check if user has IMAP settings.
			imapSettings, err := models.GetIMAP(uid)
			if err != nil {
				log.Error(err)
				break
			}
			if len(imapSettings) > 0 {
				im := imapSettings[0]
				// 3. Check if IMAP is enabled
				if im.Enabled {
					log.Debug("Checking IMAP for user ", uid, ": ", im.Username, " -> ", im.Host)
					if checkErr := checkForNewEmails(im); checkErr != nil && errors.Is(checkErr, ErrIMAPLogin) {
						consecutiveErrors++
					} else {
						consecutiveErrors = 0
					}

					sleepDur := time.Duration(im.IMAPFreq) * time.Second
					if backoff := backoffDuration(consecutiveErrors); backoff > sleepDur {
						log.Infof("IMAP login for %s has failed %d times in a row; backing off for %s", im.Username, consecutiveErrors, backoff)
						sleepDur = backoff
					}
					time.Sleep(sleepDur - 10*time.Second) // Subtract 10 to compensate for the default sleep of 10 at the bottom
				}
			}
		}
		time.Sleep(10 * time.Second)
	}
}

// NewMonitor returns a new instance of imap.Monitor
func NewMonitor() *Monitor {
	im := &Monitor{}
	return im
}

// Start launches the IMAP campaign monitor
func (im *Monitor) Start() error {
	log.Info("Starting IMAP monitor manager")
	ctx, cancel := context.WithCancel(context.Background()) // ctx is the derivedContext
	im.cancel = cancel
	go im.start(ctx)
	return nil
}

// Shutdown attempts to gracefully shutdown the IMAP monitor.
func (im *Monitor) Shutdown() error {
	log.Info("Shutting down IMAP monitor manager")
	im.cancel()
	return nil
}

// checkForNewEmails logs into an IMAP account and checks unread emails for
// the rid campaign identifier.
func checkForNewEmails(im models.IMAP) error {
	im.Host = im.Host + ":" + strconv.Itoa(int(im.Port)) // Append port
	mailServer := Mailbox{
		Host:             im.Host,
		TLS:              im.TLS,
		IgnoreCertErrors: im.IgnoreCertErrors,
		User:             im.Username,
		Pwd:              im.Password,
		Folder:           im.Folder,
		RestrictDomain:   im.RestrictDomain,
	}

	if im.AuthType == models.IMAPAuthTypeOAuth2 {
		token, err := models.GetValidAccessToken(context.Background(), &im)
		if err != nil {
			log.Error("Unable to get a valid OAuth2 access token for user ", im.UserId, ": ", err.Error())
			return err
		}
		mailServer.OAuthToken = token
	}

	msgs, err := mailServer.GetUnread(true, false)
	if err != nil {
		if errors.Is(err, ErrIMAPLogin) {
			if rerr := models.RecordLoginError(&im, err); rerr != nil {
				log.Error(rerr)
			}
		} else {
			log.Error(err)
		}
		return err
	}
	// Update last_login (and reset the consecutive error backoff) now that
	// weve successfully logged in and fetched
	if err := models.SuccessfulLogin(&im); err != nil {
		log.Error(err)
	}

	if len(msgs) > 0 {
		log.Debugf("%d new emails for %s", len(msgs), im.Username)
		var reportingFailed []uint32 // SeqNums of emails that were unable to be reported to phishing server, mark as unread
		var deleteEmails []uint32    // SeqNums of campaign emails. If DeleteReportedCampaignEmail is true, we will delete these
		var nonCampaignCount uint32  // Emails that don't match a known campaign, for admin review
		for _, m := range msgs {
			rids, err := matchEmail(m.Email) // Search email Text, HTML, and each attachment for rid parameters

			if err != nil {
				log.Errorf("Error searching email for rids from user '%s': %s", m.From, err.Error())
				continue
			}
			if len(rids) < 1 {
				log.Infof("User '%s' reported email with subject '%s'. This is not a GoPhish campaign; you should investigate it.", m.From, m.Subject)
				nonCampaignCount++
			}
			for rid := range rids {
				log.Infof("User '%s' reported email with rid %s", m.From, rid)
				result, err := models.GetResult(rid)
				if err != nil {
					log.Error("Error reporting GoPhish email with rid ", rid, ": ", err.Error())
					reportingFailed = append(reportingFailed, m.SeqNum)
					continue
				}
				err = result.HandleEmailReport(models.EventDetails{})
				if err != nil {
					log.Error("Error updating GoPhish email with rid ", rid, ": ", err.Error())
					continue
				}
				if im.DeleteReportedCampaignEmail {
					deleteEmails = append(deleteEmails, m.SeqNum)
				}
			}

		}
		if nonCampaignCount > 0 {
			if err := models.IncrementNonCampaignEmails(&im, nonCampaignCount); err != nil {
				log.Error(err)
			}
		}
		// Check if any emails were unable to be reported, so we can mark them as unread
		if len(reportingFailed) > 0 {
			log.Debugf("Marking %d emails as unread as failed to report", len(reportingFailed))
			err := mailServer.MarkAsUnread(reportingFailed) // Set emails as unread that we failed to report to GoPhish
			if err != nil {
				log.Error("Unable to mark emails as unread: ", err.Error())
			}
		}
		// If the DeleteReportedCampaignEmail flag is set, delete reported Gophish campaign emails
		if len(deleteEmails) > 0 {
			log.Debugf("Deleting %d campaign emails", len(deleteEmails))
			err := mailServer.DeleteEmails(deleteEmails) // Delete GoPhish campaign emails.
			if err != nil {
				log.Error("Failed to delete emails: ", err.Error())
			}
		}

	} else {
		log.Debug("No new emails for ", im.Username)
	}
	return nil
}

func checkRIDs(em *email.Email, rids map[string]bool) {
	// Check Text and HTML
	emailContent := string(em.Text) + string(em.HTML)
	for _, r := range goPhishRegex.FindAllStringSubmatch(emailContent, -1) {
		newrid := r[len(r)-1]
		if !rids[newrid] {
			rids[newrid] = true
		}
	}
}

// returns a slice of gophish rid paramters found in the email HTML, Text, and attachments
func matchEmail(em *email.Email) (map[string]bool, error) {
	rids := make(map[string]bool)
	checkRIDs(em, rids)

	// Next check each attachment
	for _, a := range em.Attachments {
		ext := filepath.Ext(a.Filename)
		if a.Header.Get("Content-Type") == "message/rfc822" || ext == ".eml" {

			// Let's decode the email
			rawBodyStream := bytes.NewReader(a.Content)
			attachmentEmail, err := email.NewEmailFromReader(rawBodyStream)
			if err != nil {
				return rids, err
			}

			checkRIDs(attachmentEmail, rids)
		}
	}

	return rids, nil
}
