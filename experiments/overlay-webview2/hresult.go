package main

import "fmt"

const (
	sOK             int32 = 0
	sFalse          int32 = 1
	rpcEChangedMode int32 = -2147417850 // 0x80010106
)

// coInitStatus judges the HRESULT of CoInitializeEx. S_OK and S_FALSE (already initialized on
// this thread) are success and need a matching CoUninitialize; RPC_E_CHANGED_MODE means COM is
// already up in another apartment model, which is usable but must not be uninitialized. Any
// other negative HRESULT is a real failure.
func coInitStatus(hr int32) (uninit bool, note string, err error) {
	switch {
	case hr == rpcEChangedMode:
		return false, "COM already initialized in a different apartment mode; continuing", nil
	case hr < 0:
		return false, "", fmt.Errorf("CoInitializeEx: HRESULT 0x%08X", uint32(hr))
	default:
		return true, "", nil
	}
}
