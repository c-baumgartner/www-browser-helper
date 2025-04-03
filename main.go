package main

import (
	"fmt"
	"os"
	"os/exec"
)

func checkCodespace() bool {
	codespace, ok := os.LookupEnv("CODESPACES")
	if !ok || codespace != "true" {
		return false
	}
	return true
}

func execute(url string) error {
	browserScript, ok := os.LookupEnv("BROWSER")
	if !ok || browserScript == "" {
		return fmt.Errorf("error BROSWER environment variable not set")
	}
	cmd := exec.Command(browserScript, url)
	err := cmd.Start()
	if err != nil {
		return err
	}
	cmd.Wait()
	return nil
}

func usage() {
	fmt.Println("Test")
}

func main() {

	if ok := checkCodespace(); !ok {
		fmt.Fprintln(os.Stderr, "this program is meant to be run in a Github Codespaces environment")
		os.Exit(1)
	}

	// Get the arguments from the command line without the program name
	args := os.Args[1:]

	switch len(args) {
	case 0:
		usage()
	case 1:
		err := execute(args[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		usage()
	}

	os.Exit(0)
}
