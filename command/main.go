package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/abakum/cancelreader"
	"github.com/containerd/console"
	"github.com/mattn/go-isatty"
	"github.com/xlab/closer"
)

func main() {
	var (
		raw   bool
		once  bool
		reset = func(*bool) {}
		cmd   *exec.Cmd
		arg0  = "bash"
		arg1  = "-c"
		arg2  = "echo Press any key to continue . . .;read -rn1"
	)

	defer func() {
		reset(&raw)
		closer.Close()
	}()
	if IsCygwinTerminal(os.Stdin.Fd()) {
		ConsoleCP(&once)
	} else if runtime.GOOS == "windows" {
		arg0 = "cmd"
		arg1 = "/c"

		// arg0 = "powershell"
		// arg1 = "-command"

		arg2 = "pause"
	}
	log.SetFlags(log.Lmicroseconds | log.Lshortfile)
	log.SetPrefix("\r")

	var (
		cr  cancelreader.CancelReader
		err error
	)

	for i := 0; i < 8; i++ {
		if i%4 > 1 {
			reset(&raw)
			cmd = exec.Command(arg0)
		} else {
			reset = setRaw(&raw, reset)
			cmd = exec.Command(arg0, arg1, arg2)
		}
		log.Println(cmd)
		cr, err = cancelreader.NewReader(os.Stdin)
		if err != nil {
			panic(err)
		}

		if i < 4 {
			// <Esc> <Esc> exit<Enter> exit<Enter>
			log.Println("--without pipes", i)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			fmt.Print("\r")
			if i%4 > 1 {
				fmt.Println("Type exit<Enter>\r")
			}
			cmd.Run()
		} else {
			// <Esc> <Esc> exit<Enter> exit<Enter>
			log.Println("--with pipes", i)
			ConsoleCP(&once)

			in, err := cmd.StdinPipe()
			if err != nil {
				panic(err)
			}

			out, err := cmd.StdoutPipe()
			if err != nil {
				panic(err)
			}

			fmt.Print("\r")
			if i%4 > 1 {
				fmt.Println("Type exit<Enter>\r")
			}
			err = cmd.Start()
			if err != nil {
				panic(err)
			}

			go func() {
				_, err = io.Copy(os.Stdout, out)
				log.Println("Stdout done", i, err)
				log.Println("Cancel read stdin", i, cr.Cancel())
			}()

			_, err = io.Copy(in, cr)
			log.Println("Stdin done", i, err)

			log.Println("Wait", cmd.Wait())
			log.Println("Close", cr.Close())
		}
	}
}
func setRaw(raw *bool, old func(*bool)) (reset func(*bool)) {
	reset = old
	if *raw {
		return
	}
	var (
		err      error
		current  console.Console
		settings string
	)

	current, err = console.ConsoleFromFile(os.Stdin)
	if err == nil {
		err = current.SetRaw()
		if err == nil {
			*raw = true
			reset = func(raw *bool) {
				if *raw {
					err := current.Reset()
					log.Println("Restores the console to its original state by go", err)
				}
				*raw = err != nil
			}
			log.Println("Sets the console in raw mode by go")
			return
		}
	}

	if isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		settings, err = sttySettings()
		if err == nil {
			err = sttyMakeRaw()
			if err == nil {
				*raw = true
				reset = func(raw *bool) {
					if *raw {
						sttyReset(settings)
						log.Println("Restores the console to its original state by stty")
					}
					*raw = false
				}
				log.Println("Sets the console in raw mode by stty")
				return
			}
		}
	}
	log.Println(err)
	return
}

func sttyMakeRaw() error {
	cmd := exec.Command("stty", "raw", "-echo")
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func sttySettings() (string, error) {
	cmd := exec.Command("stty", "-g")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func sttyReset(settings string) {
	cmd := exec.Command("stty", settings)
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	log.Println("sttyReset", settings, err)
}
