//go:build windows
// +build windows

package cancelreader

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func processHandleCount(t *testing.T) uint32 {
	t.Helper()
	var count uint32
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")
	ok, _, err := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&count)))
	if ok == 0 {
		t.Fatal(err)
	}
	return count
}

func TestReadAsyncClosesEventOnReadFileError(t *testing.T) {
	r := winCancelReader{conin: windows.InvalidHandle, blockingReadSignal: make(chan struct{}, 1)}
	_, _ = r.readAsync(make([]byte, 1)) // Initialize lazy API bindings before counting.
	before := processHandleCount(t)
	for i := 0; i < 100; i++ {
		if _, err := r.readAsync(make([]byte, 1)); err == nil {
			t.Fatal("ReadFile on an invalid handle should fail")
		}
	}
	after := processHandleCount(t)
	if after > before+2 {
		t.Fatalf("failed reads leaked handles: before %d, after %d", before, after)
	}
}

func TestReadAsyncPropagatesCompletionErrorAndReleasesSignal(t *testing.T) {
	name, err := windows.UTF16PtrFromString(fmt.Sprintf(`\\.\pipe\cancelreader-test-%d-%d`, os.Getpid(), time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	server, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT, 1, 1024, 1024, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(server)
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	connect := windows.Overlapped{HEvent: event}
	if err := windows.ConnectNamedPipe(server, &connect); err != nil && err != windows.ERROR_IO_PENDING {
		t.Fatal(err)
	}
	client, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(client)
	var transferred uint32
	if err := windows.GetOverlappedResult(server, &connect, &transferred, true); err != nil {
		t.Fatal(err)
	}
	r := winCancelReader{conin: server, blockingReadSignal: make(chan struct{}, 1)}
	done := make(chan error, 1)
	go func() {
		_, err := r.readAsync(make([]byte, 1))
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(r.blockingReadSignal) == 0 {
		select {
		case err := <-done:
			t.Fatalf("read completed before cancellation: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("read did not reach GetOverlappedResult")
		}
		time.Sleep(time.Millisecond)
	}
	if err := windows.CancelIoEx(server, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
			t.Fatalf("completion error = %v; want ERROR_OPERATION_ABORTED", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled read did not return")
	}
	if len(r.blockingReadSignal) != 0 {
		t.Fatal("failed completion left the read signal occupied")
	}
}
