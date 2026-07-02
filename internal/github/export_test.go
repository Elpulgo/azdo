package github

// This file exposes unexported helpers for white-box testing from the
// github_test package. It is compiled only during go test.

// MapTimelineStateExported is a test shim for mapTimelineState.
func MapTimelineStateExported(ghStatus string) string {
	return mapTimelineState(ghStatus)
}

// MapTimelineResultExported is a test shim for mapTimelineResult.
func MapTimelineResultExported(ghConclusion string) string {
	return mapTimelineResult(ghConclusion)
}
