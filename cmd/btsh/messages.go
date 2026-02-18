package main

// commandResultMsg is sent when an async command finishes
type commandResultMsg struct {
	output string
	err    error
	// cd sets this so the model can update cwd
	newCwd string
}

// scrapeProgressMsg updates spinner label during scrape
type scrapeProgressMsg struct {
	path   string
	done   int
	total  int
	errors int
}

// scrapeDoneMsg is sent when a single scrape fetch completes
type scrapeDoneMsg struct {
	path        string
	newChildren []string
	err         error
}

// findStepMsg triggers the next find search step
type findStepMsg struct {
	path string
}

// actionDiscoveredMsg is sent when action discovery completes.
// confirm=true means an action invocation needing y/N confirmation;
// confirm=false means entering action mode (showing available actions).
type actionDiscoveredMsg struct {
	actions []ActionInfo
	output  string
	err     error
	confirm bool
	body    []byte // JSON body for confirm
}

// exportStepMsg triggers the next export fetch step
type exportStepMsg struct {
	path string
}

// actionResultMsg is sent when a POST action completes
type actionResultMsg struct {
	status int
	body   string
	err    error
}
