package main

import (
	"fmt"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const popupRoute = "/index.html#/terminal-popup"

type PopupOpenResult struct {
	Success bool   `json:"success"`
	PopupID string `json:"popupId,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PopupWindowService struct {
	mu      sync.Mutex
	app     *application.App
	counter int
}

func newPopupWindowService(app *application.App) *PopupWindowService {
	return &PopupWindowService{app: app}
}

func popupWindowOptions(name string) application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name:             name,
		Title:            "LemonSSH",
		Width:            960,
		Height:           640,
		MinWidth:         640,
		MinHeight:        400,
		Frameless:        true,
		EnableFileDrop:   true,
		BackgroundColour: application.NewRGB(20, 23, 28),
		URL:              popupRoute,
	}
}

func (s *PopupWindowService) Open(payload map[string]any) PopupOpenResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app == nil {
		return PopupOpenResult{Success: false, Error: "application is not attached"}
	}
	s.counter++
	popupID := fmt.Sprintf("popup-%d", s.counter)
	win := s.app.Window.NewWithOptions(popupWindowOptions(popupID))
	registerFileDrops(win)
	if payload == nil {
		payload = map[string]any{}
	}
	payload["popupId"] = popupID
	s.app.Event.Emit("terminal:popup-config", payload)
	_ = win
	return PopupOpenResult{Success: true, PopupID: popupID}
}
