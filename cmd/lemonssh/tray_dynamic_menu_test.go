package main

import (
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeTrayForwards records StopByRuleId calls and reports a fixed tunnel table.
type fakeTrayForwards struct {
	tunnels  []PortForwardListItem
	stopped  []string
	stopSelf func()
}

func (f *fakeTrayForwards) StopByRuleId(ruleID string) map[string]any {
	f.stopped = append(f.stopped, ruleID)
	if f.stopSelf != nil {
		f.stopSelf()
	}
	return map[string]any{"stopped": len(f.stopped)}
}

func (f *fakeTrayForwards) List() []PortForwardListItem { return f.tunnels }

func trayMenuTestService() (*TrayService, *[]string, *[]string) {
	var mainEvents []string
	var panelEvents []string
	service := &TrayService{
		language: "en",
		quit:     func() {},
	}
	service.emitMainWindow = func(name string, payload any) {
		mainEvents = append(mainEvents, name)
	}
	service.emitPanel = func(name string, payload any) {
		panelEvents = append(panelEvents, name)
	}
	return service, &mainEvents, &panelEvents
}

func TestTrayMenuPlanKeepsBaseTrioPlusQuitWithoutData(t *testing.T) {
	plan := trayMenuPlan("en", TrayMenuData{})
	if len(plan) != 5 {
		t.Fatalf("empty-data plan rows = %d, want 5 (Show/Settings/TrayPanel/sep/Quit)", len(plan))
	}
	wantActions := []trayMenuAction{trayActionShowMain, trayActionOpenSettings, trayActionOpenPanel, trayActionNone, trayActionQuit}
	for index, entry := range plan {
		if entry.action != wantActions[index] {
			t.Fatalf("plan[%d].action = %v, want %v", index, entry.action, wantActions[index])
		}
	}
	if plan[3].kind != "separator" {
		t.Fatalf("plan[3].kind = %q, want separator", plan[3].kind)
	}
	if plan[2].label != "Open Tray Panel" {
		t.Fatalf("tray panel item label = %q, want Open Tray Panel", plan[2].label)
	}
}

func TestTrayMenuPlanListsActiveSessionsAndRuleToggles(t *testing.T) {
	data := TrayMenuData{
		Sessions: []TraySessionState{
			{ID: "s1", HostLabel: "AI Box", Status: "connected"},
			{ID: "s2", Label: "connecting-host", Status: "connecting"},
			{ID: "s3", HostLabel: "gone", Status: "disconnected"},
			{ID: "", Status: "connected"},
		},
		PortForwardRules: []TrayPortForwardRuleState{
			{ID: "r1", Label: "web", Status: "active"},
			{ID: "r2", Type: "dynamic", LocalPort: 1080, Status: "inactive"},
			{ID: "", Status: "active"},
		},
	}
	plan := trayMenuPlan("en", data)

	if len(plan) != 8 {
		t.Fatalf("plan rows = %d, want 8", len(plan))
	}
	sessionsSubmenu := plan[4]
	if sessionsSubmenu.kind != "submenu" || !strings.HasPrefix(sessionsSubmenu.label, "Sessions (2)") {
		t.Fatalf("sessions submenu = %+v, want 'Sessions (2)'", sessionsSubmenu)
	}
	if len(sessionsSubmenu.children) != 2 {
		t.Fatalf("session rows = %d, want 2 (disconnected and id-less rows dropped)", len(sessionsSubmenu.children))
	}
	if sessionsSubmenu.children[0].label != "AI Box" || sessionsSubmenu.children[0].sessionID != "s1" {
		t.Fatalf("first session row = %+v", sessionsSubmenu.children[0])
	}
	if sessionsSubmenu.children[0].action != trayActionFocusSession {
		t.Fatalf("session row action = %v, want focusSession", sessionsSubmenu.children[0].action)
	}

	forwardsSubmenu := plan[5]
	if forwardsSubmenu.kind != "submenu" || forwardsSubmenu.label != "Port Forwarding" {
		t.Fatalf("forwarding submenu = %+v", forwardsSubmenu)
	}
	if len(forwardsSubmenu.children) != 2 {
		t.Fatalf("rule rows = %d, want 2", len(forwardsSubmenu.children))
	}
	web := forwardsSubmenu.children[0]
	if web.kind != "checkbox" || !web.checked || web.ruleID != "r1" {
		t.Fatalf("active rule row = %+v, want checked checkbox for r1", web)
	}
	socks := forwardsSubmenu.children[1]
	if socks.label != "SOCKS:1080" || socks.checked {
		t.Fatalf("dynamic rule row = %+v, want unchecked SOCKS:1080", socks)
	}
	if socks.action != trayActionToggleForward {
		t.Fatalf("rule row action = %v, want toggleForward", socks.action)
	}
}

func TestTrayMenuPlanFallsBackToRulePortLabel(t *testing.T) {
	plan := trayMenuPlan("zh-CN", TrayMenuData{
		PortForwardRules: []TrayPortForwardRuleState{
			{ID: "r1", Type: "local", LocalPort: 8080, RemoteHost: "db.internal", RemotePort: 5432, Status: "active"},
		},
	})
	// Rules only: the forwarding submenu directly follows the shared separator.
	forwardsSubmenu := plan[4]
	if forwardsSubmenu.kind != "submenu" || forwardsSubmenu.label != "端口转发" {
		t.Fatalf("zh-CN forwarding submenu = %+v, want 端口转发", forwardsSubmenu)
	}
	if got := forwardsSubmenu.children[0].label; got != "8080 → db.internal:5432" {
		t.Fatalf("rule fallback label = %q, want 8080 → db.internal:5432", got)
	}
}

func TestTrayBuildMenuMaterializesPlanStructure(t *testing.T) {
	service, _, _ := trayMenuTestService()
	service.data = normalizeTrayMenuData(TrayMenuData{
		Sessions:         []TraySessionState{{ID: "s1", HostLabel: "AI Box", Status: "connected"}},
		PortForwardRules: []TrayPortForwardRuleState{{ID: "r1", Label: "web", Status: "active"}},
	})

	menu := service.buildMenu()
	if got := menu.ItemAt(0).Label(); got != "Open Main Window" {
		t.Fatalf("menu[0] = %q, want Open Main Window", got)
	}
	if got := menu.ItemAt(2).Label(); got != "Open Tray Panel" {
		t.Fatalf("menu[2] = %q, want Open Tray Panel", got)
	}
	if !menu.ItemAt(3).IsSeparator() {
		t.Fatal("menu[3] must be a separator before the dynamic sections")
	}
	sessionsItem := menu.ItemAt(4)
	if !sessionsItem.IsSubmenu() {
		t.Fatal("menu[4] must be the sessions submenu")
	}
	if got := sessionsItem.GetSubmenu().ItemAt(0).Label(); got != "AI Box" {
		t.Fatalf("session row label = %q, want AI Box", got)
	}
	forwardsItem := menu.ItemAt(5)
	if !forwardsItem.IsSubmenu() {
		t.Fatal("menu[5] must be the forwarding submenu")
	}
	ruleItem := forwardsItem.GetSubmenu().ItemAt(0)
	if !ruleItem.IsCheckbox() || !ruleItem.Checked() {
		t.Fatalf("rule row = checkbox=%v checked=%v, want checked checkbox", ruleItem.IsCheckbox(), ruleItem.Checked())
	}
	if !menu.ItemAt(6).IsSeparator() {
		t.Fatal("menu[6] must be a separator before Quit")
	}
	if got := menu.ItemAt(7).Label(); got != "Quit" {
		t.Fatalf("menu[7] = %q, want Quit", got)
	}
}

func TestUpdateTrayMenuDataStoresSnapshotAndMirrorsToPanel(t *testing.T) {
	service, _, panelEvents := trayMenuTestService()

	result, err := service.UpdateTrayMenuData(TrayMenuData{
		Sessions: []TraySessionState{{ID: "s1", HostLabel: "AI Box", Status: "connected"}},
		Hosts:    []TrayHostState{{ID: "h1", Label: "AI Box"}},
		PortForwardRules: []TrayPortForwardRuleState{
			{ID: "r1", Label: "web", Status: "active"},
			{ID: "", Status: "active"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateTrayMenuData failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("UpdateTrayMenuData result = %+v, want success", result)
	}

	current := service.CurrentTrayMenuData()
	if len(current.Sessions) != 1 || len(current.Hosts) != 1 || len(current.PortForwardRules) != 1 {
		t.Fatalf("stored snapshot = %+v, want id-less rule dropped", current)
	}
	if len(*panelEvents) != 2 || (*panelEvents)[0] != trayPanelMenuDataEvent || (*panelEvents)[1] != trayPanelRefreshEvent {
		t.Fatalf("panel events = %v, want [%s %s]", *panelEvents, trayPanelMenuDataEvent, trayPanelRefreshEvent)
	}

	// A second push with no sessions must rebuild the menu down to the base rows.
	if _, err := service.UpdateTrayMenuData(TrayMenuData{}); err != nil {
		t.Fatalf("second UpdateTrayMenuData failed: %v", err)
	}
	menu := service.buildMenu()
	if got := menu.ItemAt(3).Label(); got != "" || !menu.ItemAt(3).IsSeparator() {
		t.Fatalf("empty-data menu[3] = %q (separator=%v), want a separator", got, menu.ItemAt(3).IsSeparator())
	}
	if got := menu.ItemAt(4).Label(); got != "Quit" {
		t.Fatalf("empty-data menu[4] = %q, want Quit (dynamic sections gone)", got)
	}
}

func TestTrayFocusSessionRestoresMainWindowAndEmits(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	showMainCalled := 0
	service.actions.ShowMain = func() { showMainCalled++ }

	if _, err := service.FocusSession("s1"); err != nil {
		t.Fatalf("FocusSession failed: %v", err)
	}
	if showMainCalled != 1 {
		t.Fatalf("ShowMain calls = %d, want 1", showMainCalled)
	}
	if len(*mainEvents) != 1 || (*mainEvents)[0] != trayFocusSessionEvent {
		t.Fatalf("main events = %v, want [%s]", *mainEvents, trayFocusSessionEvent)
	}
	if _, err := service.FocusSession("  "); err == nil {
		t.Fatal("FocusSession must reject an empty session id")
	}
}

func TestTrayPanelJumpSharesFocusPipelineWithMenuClick(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	showMainCalled := 0
	service.actions.ShowMain = func() { showMainCalled++ }

	if _, err := service.JumpToSessionFromPanel("s1"); err != nil {
		t.Fatalf("JumpToSessionFromPanel failed: %v", err)
	}
	if showMainCalled != 1 {
		t.Fatalf("ShowMain calls = %d, want 1", showMainCalled)
	}
	if len(*mainEvents) != 1 || (*mainEvents)[0] != trayPanelJumpSessionEvent {
		t.Fatalf("main events = %v, want [%s]", *mainEvents, trayPanelJumpSessionEvent)
	}
}

func TestTrayPanelConnectAndCloseEmitToMainWindow(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	showMainCalled := 0
	service.actions.ShowMain = func() { showMainCalled++ }

	if _, err := service.ConnectToHost("h1"); err != nil {
		t.Fatalf("ConnectToHost failed: %v", err)
	}
	if _, err := service.CloseSessionFromPanel("s1"); err != nil {
		t.Fatalf("CloseSessionFromPanel failed: %v", err)
	}
	if showMainCalled != 1 {
		t.Fatalf("ShowMain calls = %d, want 1 (close must not steal focus)", showMainCalled)
	}
	if len(*mainEvents) != 2 || (*mainEvents)[0] != trayPanelConnectHostEvent || (*mainEvents)[1] != trayPanelCloseSessionEvent {
		t.Fatalf("main events = %v, want [%s %s]", *mainEvents, trayPanelConnectHostEvent, trayPanelCloseSessionEvent)
	}
}

func TestTrayTogglePortForwardStopsLiveTunnelNatively(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	forwards := &fakeTrayForwards{tunnels: []PortForwardListItem{{RuleID: "r1", TunnelID: "pf-r1-1", Status: "active"}}}
	service.setForwards(forwards)

	result, err := service.TogglePortForward("r1")
	if err != nil {
		t.Fatalf("TogglePortForward failed: %v", err)
	}
	if !result.Success || result.Running {
		t.Fatalf("stop toggle result = %+v, want success with running=false", result)
	}
	if len(forwards.stopped) != 1 || forwards.stopped[0] != "r1" {
		t.Fatalf("stopped rules = %v, want [r1]", forwards.stopped)
	}
	if len(*mainEvents) != 0 {
		t.Fatalf("stop toggle must not ask the renderer, got events %v", *mainEvents)
	}
}

func TestTrayTogglePortForwardDelegatesStartToRenderer(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	service.emitMainWindow = func(name string, payload any) {
		*mainEvents = append(*mainEvents, name)
		if name == trayToggleForwardEvent {
			data, ok := payload.(map[string]any)
			if !ok {
				t.Fatalf("toggle payload type = %T, want map", payload)
			}
			if data["ruleId"] != "r2" || data["start"] != true {
				t.Fatalf("toggle payload = %v, want ruleId=r2 start=true", data)
			}
		}
	}
	forwards := &fakeTrayForwards{tunnels: []PortForwardListItem{{RuleID: "r1"}}}
	service.setForwards(forwards)

	result, err := service.TogglePortForward("r2")
	if err != nil {
		t.Fatalf("TogglePortForward failed: %v", err)
	}
	if !result.Success || !result.Running {
		t.Fatalf("start toggle result = %+v, want success with running=true", result)
	}
	if len(forwards.stopped) != 0 {
		t.Fatalf("start toggle must not stop anything, got %v", forwards.stopped)
	}
	if len(*mainEvents) != 1 || (*mainEvents)[0] != trayToggleForwardEvent {
		t.Fatalf("main events = %v, want [%s]", *mainEvents, trayToggleForwardEvent)
	}
}

func TestTrayMenuClickHandlersInvokeServiceActions(t *testing.T) {
	service, mainEvents, _ := trayMenuTestService()
	var calls []string
	service.actions.ShowMain = func() { calls = append(calls, "showMain") }
	service.actions.OpenSettings = func() { calls = append(calls, "openSettings") }
	service.actions.OpenTrayPanel = func() { calls = append(calls, "openPanel") }
	quitCalls := 0
	service.quit = func() { quitCalls++ }
	forwards := &fakeTrayForwards{tunnels: []PortForwardListItem{{RuleID: "r1", Status: "active"}}}
	service.setForwards(forwards)

	// The handlers below are exactly the ones bindTrayMenuEntry attaches to
	// the menu items; drive them the way a native click would.
	plan := trayMenuPlan("en", TrayMenuData{
		Sessions:         []TraySessionState{{ID: "s1", HostLabel: "AI Box", Status: "connected"}},
		PortForwardRules: []TrayPortForwardRuleState{{ID: "r1", Label: "web", Status: "active"}},
	})
	handlers := map[string]func(*application.Context){}
	for _, entry := range plan {
		switch entry.kind {
		case "submenu":
			for _, child := range entry.children {
				if handler := service.trayMenuHandler(child); handler != nil {
					handlers[string(child.action)+":"+child.sessionID+child.ruleID] = handler
				}
			}
		default:
			if handler := service.trayMenuHandler(entry); handler != nil {
				handlers[string(entry.action)] = handler
			}
		}
	}

	handlers[string(trayActionShowMain)](nil)
	handlers[string(trayActionOpenSettings)](nil)
	handlers[string(trayActionOpenPanel)](nil)
	handlers[string(trayActionFocusSession)+":s1"](nil)
	handlers[string(trayActionToggleForward)+":r1"](nil)
	handlers[string(trayActionQuit)](nil)

	if strings.Join(calls, ",") != "showMain,openSettings,openPanel,showMain" {
		// The session row restores the main window before emitting, so the
		// click handler records a second showMain on purpose.
		t.Fatalf("launcher calls = %v, want showMain,openSettings,openPanel,showMain", calls)
	}
	if len(forwards.stopped) != 1 || forwards.stopped[0] != "r1" {
		t.Fatalf("stopped rules = %v, want [r1]", forwards.stopped)
	}
	// Session click: ShowMain (restore) + trayFocusSessionEvent → 1 event.
	if len(*mainEvents) != 1 || (*mainEvents)[0] != trayFocusSessionEvent {
		t.Fatalf("main events = %v, want [%s]", *mainEvents, trayFocusSessionEvent)
	}
	if quitCalls != 1 {
		t.Fatalf("quit calls = %d, want 1", quitCalls)
	}
}

func TestTrayPanelWindowServiceNilAppFailsClosed(t *testing.T) {
	service := newTrayPanelWindowService(nil)
	if _, err := service.Open(); err == nil {
		t.Fatal("Open without an attached application must fail")
	}
	if ok, err := service.PaintReady(); ok || err != nil {
		t.Fatalf("PaintReady without an app = (%v, %v), want (false, nil)", ok, err)
	}
	if ok, err := service.Hide(); ok || err != nil {
		t.Fatalf("Hide without an app = (%v, %v), want (false, nil)", ok, err)
	}
	service.Preload() // must not panic
}

func TestTrayPanelWindowOptionsAreDedicatedFramelessAndHidden(t *testing.T) {
	options := trayPanelWindowOptions()
	if options.Name != trayPanelWindowName {
		t.Fatalf("panel window name = %q, want %q", options.Name, trayPanelWindowName)
	}
	if options.URL != "/index.html#/tray" {
		t.Fatalf("panel window URL = %q, want /index.html#/tray", options.URL)
	}
	if !options.Frameless || !options.Hidden || !options.AlwaysOnTop {
		t.Fatalf("panel window must be frameless, hidden and always-on-top, got %+v", options)
	}
	if !options.Windows.HiddenOnTaskbar {
		t.Fatal("panel window must hide from the taskbar on Windows")
	}
	if options.Width != 360 || options.Height != 520 {
		t.Fatalf("panel window size = %dx%d, want 360x520", options.Width, options.Height)
	}
}

func TestTrayPanelWindowShownAndHideCallbacksFire(t *testing.T) {
	service := newTrayPanelWindowService(nil)
	shown := 0
	hideRequests := 0
	service.setPanelShown(func() { shown++ })
	service.setPanelHideRequest(func() { hideRequests++ })

	// The callbacks are captured when the window is created; with no app the
	// window never exists, so exercise the wiring contract via the fields the
	// registration reads.
	if service.onPanelShown == nil || service.onPanelHideRequest == nil {
		t.Fatal("setPanelShown/setPanelHideRequest must store their callbacks")
	}
	service.onPanelShown()
	service.onPanelHideRequest()
	if shown != 1 || hideRequests != 1 {
		t.Fatalf("callbacks fired shown=%d hideRequests=%d, want 1/1", shown, hideRequests)
	}
}
