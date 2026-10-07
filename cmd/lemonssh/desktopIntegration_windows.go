//go:build windows

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

func setJMSProtocolEnabled(enabled bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	root := `Software\Classes\jms`
	if !enabled {
		for _, key := range []string{root + `\shell\open\command`, root + `\shell\open`, root + `\shell`, root} {
			if err := registry.DeleteKey(registry.CURRENT_USER, key); err != nil && err != registry.ErrNotExist {
				return err
			}
		}
		return nil
	}
	values := []struct{ key, name, value string }{
		{root, "", "URL:JMS Protocol"}, {root, "URL Protocol", ""},
		{root + `\shell\open\command`, "", fmt.Sprintf(`"%s" "%%1"`, executable)},
	}
	for _, item := range values {
		key, _, createErr := registry.CreateKey(registry.CURRENT_USER, item.key, registry.SET_VALUE|registry.QUERY_VALUE)
		if createErr != nil {
			return createErr
		}
		setErr := key.SetStringValue(item.name, item.value)
		key.Close()
		if setErr != nil {
			return setErr
		}
	}
	return nil
}

func jmsProtocolEnabled() bool {
	executable, err := os.Executable()
	if err != nil {
		return false
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\jms\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetStringValue("")
	return err == nil && value == fmt.Sprintf(`"%s" "%%1"`, executable)
}

func setExplorerContextMenu(enabled bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	roots := []struct{ path, placeholder string }{{`Software\Classes\Directory\Background\shell\LemonSSH`, "%V"}, {`Software\Classes\Directory\shell\LemonSSH`, "%1"}}
	for _, item := range roots {
		root := item.path
		if !enabled {
			for _, key := range []string{root + `\command`, root} {
				if err := registry.DeleteKey(registry.CURRENT_USER, key); err != nil && err != registry.ErrNotExist {
					return err
				}
			}
			continue
		}
		key, _, createErr := registry.CreateKey(registry.CURRENT_USER, root, registry.SET_VALUE)
		if createErr != nil {
			return createErr
		}
		_ = key.SetStringValue("", "Open LemonSSH Here")
		_ = key.SetStringValue("Icon", executable)
		key.Close()
		commandKey, _, createErr := registry.CreateKey(registry.CURRENT_USER, root+`\command`, registry.SET_VALUE|registry.QUERY_VALUE)
		if createErr != nil {
			return createErr
		}
		setErr := commandKey.SetStringValue("", fmt.Sprintf(`"%s" --open-terminal "%s"`, executable, item.placeholder))
		commandKey.Close()
		if setErr != nil {
			return setErr
		}
	}
	return nil
}

func explorerContextMenuEnabled() (bool, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\Directory\Background\shell\LemonSSH\command`, registry.QUERY_VALUE)
	if err != nil {
		return false, true
	}
	defer key.Close()
	_, _, err = key.GetStringValue("")
	return err == nil, true
}
