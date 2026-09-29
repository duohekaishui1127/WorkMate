//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type taskbarGUID struct {
	Data1        uint32
	Data2, Data3 uint16
	Data4        [8]byte
}

// The method offsets below come from Microsoft's UIAutomationClient.h.
// Only button rectangles are read; application names and contents are not queried.
type taskbarCOM struct{ vtable *[64]uintptr }

//go:uintptrescapes
func (c *taskbarCOM) call(method int, args ...uintptr) uintptr {
	all := append([]uintptr{uintptr(unsafe.Pointer(c))}, args...)
	r, _, _ := syscall.SyscallN(c.vtable[method], all...)
	runtime.KeepAlive(c)
	return r
}

func (c *taskbarCOM) release() {
	if c != nil {
		c.call(2)
	}
}
func taskbarCOMOK(hr uintptr) bool { return int32(hr) >= 0 }

type taskbarAutomationRequest struct{ root, parent HWND }
type taskbarAutomationResult struct {
	request taskbarAutomationRequest
	rects   []RECT
	at      time.Time
	ok      bool
}

var taskbarAutomation struct {
	once        sync.Once
	mu          sync.Mutex
	requests    chan taskbarAutomationRequest
	lastRequest time.Time
	result      taskbarAutomationResult
}

// A dedicated MTA thread keeps accessibility calls out of the window/message thread.
func cachedTaskbarButtons(root, parent HWND) ([]RECT, bool) {
	taskbarAutomation.once.Do(func() {
		taskbarAutomation.requests = make(chan taskbarAutomationRequest, 1)
		go taskbarAutomationWorker()
	})
	now := time.Now()
	request := taskbarAutomationRequest{root, parent}
	taskbarAutomation.mu.Lock()
	defer taskbarAutomation.mu.Unlock()
	result := taskbarAutomation.result
	if result.request != request || now.Sub(taskbarAutomation.lastRequest) >= 500*time.Millisecond {
		select {
		case taskbarAutomation.requests <- request:
			taskbarAutomation.lastRequest = now
		default:
		}
	}
	if result.request != request || !result.ok || now.Sub(result.at) > 2*time.Second {
		return nil, false
	}
	return append([]RECT(nil), result.rects...), true
}

func taskbarAutomationWorker() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	initialized, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0) // COINIT_MULTITHREADED
	var automation *taskbarCOM
	if taskbarCOMOK(initialized) {
		defer ole32.NewProc("CoUninitialize").Call()
		class := taskbarGUID{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
		iid := taskbarGUID{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
		hr, _, _ := ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&class)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&automation)))
		if !taskbarCOMOK(hr) {
			automation = nil
		}
	}
	defer automation.release()
	for request := range taskbarAutomation.requests {
		rects, ok := readTaskbarButtons(automation, request.root)
		taskbarAutomation.mu.Lock()
		taskbarAutomation.result = taskbarAutomationResult{request, rects, time.Now(), ok}
		taskbarAutomation.mu.Unlock()
	}
}

func readTaskbarButtons(automation *taskbarCOM, root HWND) ([]RECT, bool) {
	if automation == nil || root == 0 {
		return nil, false
	}
	var element, condition, array *taskbarCOM
	if !taskbarCOMOK(automation.call(6, uintptr(root), uintptr(unsafe.Pointer(&element)))) || element == nil {
		return nil, false
	}
	defer element.release()
	if !taskbarCOMOK(automation.call(21, uintptr(unsafe.Pointer(&condition)))) || condition == nil {
		return nil, false
	}
	defer condition.release()
	if !taskbarCOMOK(element.call(6, 4, uintptr(unsafe.Pointer(condition)), uintptr(unsafe.Pointer(&array)))) || array == nil {
		return nil, false
	} // TreeScope_Descendants
	defer array.release()
	var count int32
	if !taskbarCOMOK(array.call(3, uintptr(unsafe.Pointer(&count)))) || count < 0 || count > 512 {
		return nil, false
	}
	var rects []RECT
	for i := int32(0); i < count; i++ {
		var child *taskbarCOM
		if !taskbarCOMOK(array.call(4, uintptr(i), uintptr(unsafe.Pointer(&child)))) || child == nil {
			return nil, false
		}
		var kind int32
		var rect RECT
		ok := taskbarCOMOK(child.call(21, uintptr(unsafe.Pointer(&kind))))
		if ok && kind == 50000 {
			ok = taskbarCOMOK(child.call(43, uintptr(unsafe.Pointer(&rect))))
		}
		child.release()
		if !ok {
			return nil, false
		}
		if kind == 50000 && rect.Right > rect.Left && rect.Bottom > rect.Top {
			rects = append(rects, rect)
		}
	}
	return rects, len(rects) > 0
}
