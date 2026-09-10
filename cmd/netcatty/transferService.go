package main

import "github.com/binaricat/netcatty/internal/terminal/transfer"

type TransferService struct {
	scheduler *transfer.Scheduler
}

func newTransferService() *TransferService {
	return &TransferService{scheduler: transfer.New()}
}

func (s *TransferService) Enqueue(spec transfer.TaskSpec) (transfer.Progress, error) {
	return s.scheduler.Enqueue(spec)
}

func (s *TransferService) Pause(taskID string) error {
	return s.scheduler.Pause(taskID)
}

func (s *TransferService) Resume(taskID string) error {
	return s.scheduler.Resume(taskID)
}

func (s *TransferService) Cancel(taskID string) error {
	return s.scheduler.Cancel(taskID)
}

func (s *TransferService) Progress(taskID string) (transfer.Progress, error) {
	return s.scheduler.Progress(taskID)
}
