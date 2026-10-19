package ldap

import (
	"context"
	"time"

	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
)

// managerPollInterval is how often the manager scans for newly created or
// deleted sync mappings, matching the IMAP monitor's 10s discovery loop.
const managerPollInterval = 10 * time.Second

// Monitor is a background worker that periodically synchronizes
// LDAP-managed groups. It follows the same shape as imap/monitor.go: a
// manager goroutine discovers enabled sync mappings and spawns one worker
// goroutine per mapping, each running on that mapping's own interval.
type Monitor struct {
	cancel func()
}

// NewMonitor returns a new, unstarted Monitor.
func NewMonitor() *Monitor {
	return &Monitor{}
}

// Start launches the LDAP sync monitor manager.
func (m *Monitor) Start() error {
	log.Info("Starting LDAP sync monitor manager")
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	go m.start(ctx)
	return nil
}

// Shutdown gracefully stops the monitor manager.
func (m *Monitor) Shutdown() error {
	log.Info("Shutting down LDAP sync monitor manager")
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// start discovers enabled sync mappings and ensures exactly one worker
// goroutine is running per mapping id.
func (m *Monitor) start(ctx context.Context) {
	running := make(map[int64]int) // mapping id -> sentinel

	for {
		select {
		case <-ctx.Done():
			return
		default:
			syncs, err := models.GetAllEnabledLDAPGroupSyncs()
			if err != nil {
				log.Error(err)
				break
			}
			for _, s := range syncs {
				if _, ok := running[s.Id]; !ok {
					log.Info("Starting new LDAP sync worker for mapping ", s.Id)
					running[s.Id] = 1
					go worker(s.Id, ctx)
				}
			}
			time.Sleep(managerPollInterval)
		}
	}
}

// worker continuously synchronizes a single mapping until it is deleted or
// disabled, or the context is cancelled. It re-reads the mapping each cycle
// so configuration/interval changes take effect without a restart.
func worker(id int64, ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			s, err := models.GetLDAPGroupSyncByID(id)
			if err != nil {
				log.Info("LDAP sync mapping ", id, " seems to have been deleted. Stopping its worker.")
				return
			}
			if !s.Enabled {
				// Disabled: stop this worker. The manager will relaunch it
				// if it is re-enabled.
				log.Info("LDAP sync mapping ", id, " is disabled. Stopping its worker.")
				return
			}

			if _, serr := Sync(&s); serr != nil {
				// Error is already recorded on the mapping by Sync; log and
				// continue so transient directory outages self-heal.
				log.Errorf("LDAP sync for mapping %d failed: %s", id, serr.Error())
			}

			freq := s.SyncFreq
			if freq < models.MinLDAPSyncFreq {
				freq = models.DefaultLDAPSyncFreq
			}
			sleep := time.Duration(freq) * time.Second

			// Sleep in short slices so cancellation/disable is responsive.
			deadline := time.Now().Add(sleep)
			for time.Now().Before(deadline) {
				select {
				case <-ctx.Done():
					return
				default:
					time.Sleep(managerPollInterval)
				}
			}
		}
	}
}
