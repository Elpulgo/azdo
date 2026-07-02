package provider

import "fmt"

// PartialError indicates that some (but not all) sources failed during a
// multi-source fetch. The caller receives valid data from the successful
// sources alongside this error.
//
// The github.MultiClient (task 12) and any future multi-source fan-out must
// use this type — not redefine their own — so that callers can do a single
// errors.As check regardless of backend.
type PartialError struct {
	Failed int     // number of sources that failed
	Total  int     // total number of sources
	Errors []error // individual source errors
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("%d of %d sources failed to load", e.Failed, e.Total)
}
