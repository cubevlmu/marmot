package utils

import (
	"unsafe"
)

func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

func StringToBytes(s string) (b []byte) {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
