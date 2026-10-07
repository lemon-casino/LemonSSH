//go:build darwin && cgo

package applock

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Foundation -framework LocalAuthentication
#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#include <stdlib.h>
#include <string.h>
static int touchIDAvailable(void) {
 @autoreleasepool { LAContext *context = [[LAContext alloc] init]; NSError *error = nil;
 BOOL available = [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&error]; [context release]; return available; }
}
static char *authenticateTouchID(void) {
 @autoreleasepool {
  LAContext *context = [[LAContext alloc] init];
  context.localizedFallbackTitle = @"";
  NSError *error = nil;
  if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&error]) {
   char *result = strdup([[error localizedDescription] UTF8String] ?: "Touch ID unavailable");
   [context release]; return result;
  }
  dispatch_semaphore_t done = dispatch_semaphore_create(0);
  __block BOOL authenticated = NO;
  __block NSString *failure = nil;
  [context evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics localizedReason:@"Unlock LemonSSH" reply:^(BOOL success, NSError *err) {
   authenticated = success; failure = [[err localizedDescription] copy]; dispatch_semaphore_signal(done);
  }];
  long status = dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC));
  if (status != 0) { [context invalidate]; dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER); }
  char *result = NULL;
  if (status != 0) result = strdup("Touch ID authentication timed out");
  else if (!authenticated) result = strdup([failure UTF8String] ?: "Touch ID authentication rejected");
  [failure release]; [context release]; dispatch_release(done); return result;
 }
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func BiometricAvailable() error {
	if C.touchIDAvailable() == 0 {
		return fmt.Errorf("Touch ID is not configured or available")
	}
	return nil
}

func AuthenticateBiometric() error {
	message := C.authenticateTouchID()
	if message == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(message))
	return fmt.Errorf("Touch ID: %s", C.GoString(message))
}
