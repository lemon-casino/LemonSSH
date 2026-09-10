package main

import (
	"strings"

	"github.com/binaricat/netcatty/internal/platform/deeplink"
)

type DeepLinkService struct {
	queue *deeplink.Queue
}

func newDeepLinkService() *DeepLinkService {
	return &DeepLinkService{queue: deeplink.NewQueue()}
}

func (s *DeepLinkService) Parse(rawURL string) (*deeplink.Action, error) {
	return deeplink.Parse(rawURL)
}

func (s *DeepLinkService) Enqueue(rawURL string) error {
	return s.queue.Enqueue(rawURL)
}

func (s *DeepLinkService) Pending() int {
	return s.queue.Pending()
}

func (s *DeepLinkService) Ready() {
	s.queue.Ready()
}

func (s *DeepLinkService) Drain() []*deeplink.Action {
	return s.queue.Drain()
}

func deepLinkURLsFromArgs(args []string) []string {
	var urls []string
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "telnet://") || strings.HasPrefix(lower, "netcatty://") {
			urls = append(urls, arg)
		}
	}
	return urls
}
