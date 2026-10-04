package macos

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Foundation -framework Security
#include <stdlib.h>
#include "keychain.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// ErrNotInKeychain means there's no such item in the keychain.
var ErrNotInKeychain = errors.New("not in the keychain")

// errSecItemNotFound is the Security framework's status for a missing item.
const errSecItemNotFound = -25300

// KeychainSet stores secret in the login keychain under service and
// account, replacing any value already there. Keychain Access shows it
// as label.
func KeychainSet(service, account, label, secret string) error {
	cs, ca, cl, csec := C.CString(service), C.CString(account), C.CString(label), C.CString(secret)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	defer C.free(unsafe.Pointer(cl))
	defer C.free(unsafe.Pointer(csec))
	return keychainErr(C.lsl_keychain_set(cs, ca, cl, csec))
}

// KeychainGet returns the secret stored under service and account, or
// ErrNotInKeychain.
func KeychainGet(service, account string) (string, error) {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	var out *C.char
	if err := keychainErr(C.lsl_keychain_get(cs, ca, &out)); err != nil {
		return "", err
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out), nil
}

// KeychainDelete removes the item stored under service and account. A
// missing item isn't an error.
func KeychainDelete(service, account string) error {
	cs, ca := C.CString(service), C.CString(account)
	defer C.free(unsafe.Pointer(cs))
	defer C.free(unsafe.Pointer(ca))
	return keychainErr(C.lsl_keychain_delete(cs, ca))
}

func keychainErr(status C.int) error {
	switch status {
	case 0:
		return nil
	case errSecItemNotFound:
		return ErrNotInKeychain
	}
	msg := C.lsl_keychain_message(status)
	defer C.free(unsafe.Pointer(msg))
	return fmt.Errorf("keychain: %s (%d)", C.GoString(msg), int(status))
}
