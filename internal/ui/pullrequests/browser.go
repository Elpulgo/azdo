package pullrequests

import "github.com/Elpulgo/azdo/internal/browser"

// openBrowser is the function used to open URLs in the browser.
// Replaced in tests to capture calls without launching a real browser.
var openBrowser = browser.Open
