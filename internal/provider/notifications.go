package provider

// NotificationSource is an optional capability interface for backends that
// can supply a user-level notification inbox (Decision 1, ADR 0001). It is
// deliberately a separate interface, not a set of methods added to Provider:
// Azure DevOps has no inbox API in phase 1, so it implements nothing here
// rather than carrying empty stub methods or a runtime ErrUnsupported path.
// Metrics set this precedent on Provider already.
//
// Consumers discover support via a type assertion against the concrete
// backend, not against CompositeProvider itself (which implements this
// interface unconditionally):
//
//	if src, ok := backend.(NotificationSource); ok { ... }
//
// Unlike every other Provider method, these three do NOT take a leading
// scope string (learned convention 2 does not apply here — see the spec's
// Constraints section): GET /notifications is a user-level endpoint, not
// routed to a per-repo or per-project sub-client.
type NotificationSource interface {
	// List returns the caller's notification inbox shaped by opts. opts
	// carries fetch intent only (server-side query shaping, pagination
	// hints); config-driven filtering over the result is a separate pure
	// function (task 10) and is not performed here.
	List(opts NotifOpts) ([]Notification, error)

	// MarkRead marks the given notification as read. id must be the
	// notification's own provider-qualified Identity (Decision 14), so a
	// bare native id from one backend can never be mistaken for another's.
	// Marking read is one-way: there is no corresponding mark-unread call,
	// because GitHub exposes no such endpoint (Decision 13).
	MarkRead(id Identity) error

	// MarkDone marks the given notification as done, removing it from the
	// inbox. id must be the notification's own provider-qualified Identity
	// (Decision 14).
	MarkDone(id Identity) error
}
