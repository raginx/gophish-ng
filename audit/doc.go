// Package audit provides an append-only, structured audit log for
// security-relevant events (authentication, account management, campaign
// lifecycle).
//
// Events are written as JSON Lines (one JSON object per line) to a dedicated
// file, separate from the application log.
//
// The sink is a process-global configured once via Setup and is disabled by
// default. Emitting an event is a single Record call at the relevant handler;
// when the sink is disabled, Record is a cheap no-op.
package audit
