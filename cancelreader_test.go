package cancelreader

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const testStr = "hello"

func TestReaderNonFile(t *testing.T) {
	cr, err := NewReader(strings.NewReader(""))
	if err != nil {
		t.Errorf("expected no error, but got %s", err)
	}

	if cr.Cancel() {
		t.Errorf("expected cancellation to be failure")
	}
}

// Test that a redirected stdin still works when used with with NewReader().
func TestRedirectedStdin(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = []string{"GO_TEST_MODE=reader"}
	writer, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("cmd.StdinPipe():", err)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		t.Fatal("cmd.Start():", err)
	}
	_, err = fmt.Fprintln(writer, testStr)
	if err != nil {
		t.Fatal("fmt.Fprintln():", err)
	}
	err = cmd.Wait()
	if err != nil {
		// Will fail if child process returns nonzero
		t.Fatal("cmd.Wait():", err)
	}
}

func testChildReader() int {
	r, err := NewReader(os.Stdin)
	if err != nil {
		panic(err)
	}
	ch := make(chan string)

	go func() {
		var str string
		n, err := fmt.Fscanln(r, &str)
		if err != nil {
			panic(err)
		}
		if n != 1 {
			panic("n != 1")
		}
		ch <- str
	}()

	// give up after two seconds
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case str := <-ch:
		if str != testStr {
			panic(fmt.Sprintf("[%s] != expected [%s]", str, testStr))
		}
		break
	case <-timer.C:
		panic("timeout")
	}

	return 0
}

func TestMain(m *testing.M) {
	switch os.Getenv("GO_TEST_MODE") {
	case "":
		// Test as normal
		os.Exit(m.Run())

	case "reader":
		os.Exit(testChildReader())
	}
}
