package updateuse

import "log"

// scheduleRelaunch spawns the detached relaunch helper immediately, before
// the caller shuts the application down (a time.AfterFunc would die with the
// process). The helper command itself waits a beat before starting the new
// image so the quitting process can release its locks first: the
// single-instance socket, the profile store file and the just-swapped
// executable image.
func scheduleRelaunch(executable string, startDetached func(name string, argv []string) error) {
	name, argv := relaunchCommand(executable)
	start := startDetached
	if start == nil {
		start = startDetachedProcess
	}
	if err := start(name, argv); err != nil {
		log.Printf("update: relaunch failed: %v", err)
	}
}
