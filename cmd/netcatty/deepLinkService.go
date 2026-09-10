package main

import "github.com/binaricat/netcatty/internal/platform/deeplink"

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
