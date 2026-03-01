package azdevops

import "fmt"

// PartialError indicates that some (but not all) projects failed during a
// multi-project fetch. The caller receives valid data from the successful
// projects alongside this error.
type PartialError struct {
	Failed int     // number of projects that failed
	Total  int     // total number of projects
	Errors []error // individual project errors
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("%d of %d projects failed to load", e.Failed, e.Total)
}
