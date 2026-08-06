package provider

import "time"

// NotificationSource is an optional capability interface for backends that
// can supply a user-level notification inbox (ADR 0001). It is deliberately a
// separate interface, not a set of methods added to Provider: Azure DevOps has
// no inbox API today, so it implements nothing here rather than carrying empty
// stub methods or a runtime ErrUnsupported path. Metrics set this precedent on
// Provider already.
//
// Consumers discover support via a type assertion against the concrete
// backend, not against CompositeProvider itself (which implements this
// interface unconditionally):
//
//	if src, ok := backend.(NotificationSource); ok { ... }
//
// Unlike every other Provider method, these three do NOT take a leading scope
// string (learned convention 2 does not apply here): GET /notifications is a
// user-level endpoint, not routed to a per-repo or per-project sub-client.
type NotificationSource interface {
	// List returns the caller's notification inbox shaped by opts. opts
	// carries fetch intent only (server-side query shaping, pagination
	// hints); config-driven filtering over the result is a separate pure
	// function and is not performed here.
	List(opts NotifOpts) ([]Notification, error)

	// MarkRead marks the given notification as read. id must be the
	// notification's own provider-qualified Identity, so a bare native id from
	// one backend can never be mistaken for another's. Marking read is one-way:
	// there is no corresponding mark-unread call, because GitHub exposes no
	// such endpoint.
	MarkRead(id Identity) error

	// MarkDone marks the given notification as done, removing it from the
	// inbox. id must be the notification's own provider-qualified Identity.
	MarkDone(id Identity) error
}

// PollIntervalHinter is a separate optional capability interface for backends
// that can report a server-suggested polling cadence — for GitHub, the last
// response's X-Poll-Interval header. It is deliberately not a fourth method on
// NotificationSource: Azure DevOps has no such hint, and a backend that does
// not implement PollIntervalHinter simply falls back to the configured
// interval rather than needing a stub method or a zero-value sentinel on the
// core interface.
//
// PollInterval returns 0 when no hint is available yet (e.g. before the
// first successful fetch), which callers must treat the same as "not
// implemented".
type PollIntervalHinter interface {
	PollInterval() time.Duration
}
