//go:build darwin && cgo

package deeplink

/*
#cgo LDFLAGS: -framework CoreFoundation -framework CoreServices
#include <CoreFoundation/CoreFoundation.h>
#include <CoreServices/CoreServices.h>
#include <stdlib.h>
#include <string.h>
static int registerBundle(const char *path) {
 CFURLRef url = CFURLCreateFromFileSystemRepresentation(NULL, (const UInt8*)path, strlen(path), true);
 if (!url) return -1;
 OSStatus status = LSRegisterURL(url, true);
 CFRelease(url); return status;
}
static int setScheme(const char *scheme, const char *bundle) {
 CFStringRef s = CFStringCreateWithCString(NULL, scheme, kCFStringEncodingUTF8);
 CFStringRef b = CFStringCreateWithCString(NULL, bundle, kCFStringEncodingUTF8);
 OSStatus status = LSSetDefaultHandlerForURLScheme(s, b);
 CFRelease(s); CFRelease(b); return status;
}
static int isScheme(const char *scheme, const char *bundle) {
 CFStringRef s = CFStringCreateWithCString(NULL, scheme, kCFStringEncodingUTF8);
 CFStringRef b = CFStringCreateWithCString(NULL, bundle, kCFStringEncodingUTF8);
 CFStringRef current = LSCopyDefaultHandlerForURLScheme(s);
 int same = current && CFStringCompare(current, b, kCFCompareCaseInsensitive) == kCFCompareEqualTo;
 if (current) CFRelease(current); CFRelease(s); CFRelease(b); return same;
}
*/
import "C"
import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"
)

func bundleIdentity(executable string) (string, string, error) {
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(executable)))
	if !strings.HasSuffix(bundle, ".app") || filepath.Base(filepath.Dir(executable)) != "MacOS" {
		return "", "", fmt.Errorf("protocol registration requires a packaged .app")
	}
	out, err := exec.Command("/usr/libexec/PlistBuddy", "-c", "Print :CFBundleIdentifier", filepath.Join(bundle, "Contents", "Info.plist")).Output()
	if err != nil {
		return "", "", fmt.Errorf("read bundle identity: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", "", fmt.Errorf("missing bundle identifier")
	}
	return bundle, id, nil
}
func SetNativeProtocols(executable string, enabled bool) error {
	// LaunchServices has no supported unset-default API. Never claim disable succeeded.
	if !enabled {
		return fmt.Errorf("macOS URL defaults must be reassigned in another application; unregister is unavailable")
	}
	bundle, id, err := bundleIdentity(executable)
	if err != nil {
		return err
	}
	path := C.CString(bundle)
	defer C.free(unsafe.Pointer(path))
	if status := C.registerBundle(path); status != 0 {
		return fmt.Errorf("LSRegisterURL: status %d", status)
	}
	bid := C.CString(id)
	defer C.free(unsafe.Pointer(bid))
	for _, scheme := range ProtocolSchemes {
		s := C.CString(scheme)
		status := C.setScheme(s, bid)
		C.free(unsafe.Pointer(s))
		if status != 0 {
			return fmt.Errorf("LSSetDefaultHandlerForURLScheme %s: status %d", scheme, status)
		}
	}
	ok, err := NativeProtocolsRegistered(executable)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("LaunchServices did not accept protocol defaults")
	}
	return nil
}
func NativeProtocolsRegistered(executable string) (bool, error) {
	_, id, err := bundleIdentity(executable)
	if err != nil {
		return false, err
	}
	bid := C.CString(id)
	defer C.free(unsafe.Pointer(bid))
	for _, scheme := range ProtocolSchemes {
		s := C.CString(scheme)
		same := C.isScheme(s, bid)
		C.free(unsafe.Pointer(s))
		if same == 0 {
			return false, nil
		}
	}
	return true, nil
}
