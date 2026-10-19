package provider

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTag is wrapped by every error NormalizeTag returns, so callers can
// tell a validation failure apart from a backend error with errors.Is.
var ErrInvalidTag = errors.New("invalid tag")

// NormalizeTag trims leading and trailing whitespace from raw and validates
// the result as a single tag (Azure DevOps) or label (GitHub) for an item of
// the given kind. The trimmed value is returned unchanged otherwise: case is
// kept as typed and nothing is stripped silently.
//
// Allowed characters are 0-9, a-z, A-Z, å ä ö Å Ä Ö, '-' and '_'. KindAzure
// additionally allows ' '. Separators ('-', '_' and the Azure space) must
// have a letter or digit on both sides, so they can never be first, last or
// doubled.
func NormalizeTag(raw string, kind Kind) (string, error) {
	tag := strings.TrimSpace(raw)
	if tag == "" {
		return "", fmt.Errorf("%w: tag is empty", ErrInvalidTag)
	}

	runes := []rune(tag)
	for i, r := range runes {
		switch {
		case isTagLetterOrDigit(r):
			continue
		case r == '-' || r == '_' || r == ' ':
			if r == ' ' && kind != KindAzure {
				return "", fmt.Errorf("%w: spaces are only allowed in Azure DevOps tags", ErrInvalidTag)
			}
			if i == 0 || i == len(runes)-1 || !isTagLetterOrDigit(runes[i-1]) || !isTagLetterOrDigit(runes[i+1]) {
				return "", fmt.Errorf("%w: %q must be between letters or digits", ErrInvalidTag, r)
			}
		default:
			return "", fmt.Errorf("%w: character %q is not allowed", ErrInvalidTag, r)
		}
	}
	return tag, nil
}

func isTagLetterOrDigit(r rune) bool {
	switch {
	case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		return true
	}
	return strings.ContainsRune("åäöÅÄÖ", r)
}
