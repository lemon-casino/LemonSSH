//go:build !windows

package main

import "fmt"

func setJMSProtocolEnabled(bool) error {
	return fmt.Errorf("JMS protocol registration is unavailable on this platform")
}
func jmsProtocolEnabled() bool                 { return false }
func setExplorerContextMenu(bool) error        { return fmt.Errorf("Explorer context menu is Windows-only") }
func explorerContextMenuEnabled() (bool, bool) { return false, false }
