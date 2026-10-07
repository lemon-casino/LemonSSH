package ssh

import "time"

type keepaliveClient interface {
	SendRequest(string, bool, []byte) (bool, []byte, error)
	Close() error
}

// Only one request is in flight: an unanswered request cannot accumulate
// goroutines, and closing the client releases the blocked SSH request.
func runKeepalive(client keepaliveClient, interval time.Duration, countMax int, stop <-chan struct{}) {
	if interval <= 0 {
		return
	}
	if countMax <= 0 {
		countMax = 3
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var reply chan error
	missed := 0
	for {
		select {
		case <-stop:
			return
		case err := <-reply:
			if err != nil {
				_ = client.Close()
				return
			}
			reply = nil
			missed = 0
		case <-ticker.C:
			if reply != nil {
				missed++
				if missed >= countMax {
					_ = client.Close()
					return
				}
				continue
			}
			reply = make(chan error, 1)
			result := reply
			go func() { _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); result <- err }()
		}
	}
}
