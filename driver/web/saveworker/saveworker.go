// Package saveworker holds gunim's save worker: a service worker that has a
// browser write a file a gunim program saves to the disk as its bytes come,
// as a download, where the browser has no dialog for saving. Without it such
// a file is kept whole in the browser's memory until it is saved, which a
// file of a few GB does not survive.
//
// A program serves [Script] from its page's own origin, named [Name], in the
// page's folder or above it, and names it in its page:
//
//	<meta name="gunim-save-worker" content="gunim-save.js">
//
// The content is the script's address, relative to the page. The driver
// registers the worker for the folder gunim-save/ beside the script, so it
// sees no other request of the page's. A page served over plain HTTP from
// anywhere but the computer itself has no service workers, and a
// Content-Security-Policy must allow the script as a worker (worker-src
// 'self').
//
// The package holds no browser code of Go's, so a server built for any
// system embeds the script.
package saveworker

import _ "embed"

// Name is the file name the driver's page expects, as in its meta tag.
const Name = "gunim-save.js"

// Script is the worker, to serve as text/javascript.
//
//go:embed gunim-save.js
var Script []byte
