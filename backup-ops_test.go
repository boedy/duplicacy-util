// Copyright © 2018 Jeff Coffler <jeff@taltos.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Set up logging for test purposes
func setupLogging() (*log.Logger, *os.File, error) {
	// Create output log file
	file, err := ioutil.TempFile("", "taltos_log")
	if err != nil {
		logError(nil, fmt.Sprint("Error: ", err))
		return nil, nil, err
	}
	logger := log.New(file, "", 0 /* log.Ltime */)

	return logger, file, nil
}

// Set up arguments for testing of os/exec calls
func fakeBackupOpsCommand(command string, args...string) *exec.Cmd {
	cs := []string{"-test.run=TestBackupOpsHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestRunDuplicacyBackup(t *testing.T) {
	// Set up logging infrastructure
	logger, _, err := setupLogging()
	if err != nil {
		t.Errorf("unexpected error creating log file, got %#v", err)
	}
	loggingSystemDisplayTime = false
	defer func(){ loggingSystemDisplayTime = true }()

	// Initialize data structures for test
	configFile.backupInfo = []map[string]string {
		{"name": "b2", "threads": "5", "vss": "false"},
	}
	configFile.copyInfo = nil
	mailBody = nil
	//defer os.Remove(file.Name())

	execCommand = fakeBackupOpsCommand
	defer func(){ execCommand = exec.Command }()
	if err := performDuplicacyBackup(logger, []string {"testbackup", "taltos.log"}); err != nil {
		t.Errorf("Expected nil error, got %#v", err)
	}

	// Check results of anon function
	expectedOutput := "This is the expected\noutput\n"
	actualOutput := strings.Join(mailBody, "\n") + "\n"
	if actualOutput != expectedOutput { t.Errorf("result was incorrect, got '%s', expected '%s'.", actualOutput, expectedOutput) }
}

// TestBackupOpsHelperProcess isn't a real test; it's a helper process for TestRunDuplicacy*
func TestBackupOpsHelperProcess(t *testing.T){
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, args := args[0], args[1:]
	if cmd != "" {
		// For test, we don't pass a command. If one is found, just return a failure.
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(2)
	}

	switch args[0] {
	case "testbackup":
		backupFile := args[1]
		args = args[2:]
		fmt.Fprintf(os.Stdout, "Processing backup file: %q\n", backupFile)

	default:
		fmt.Fprintf(os.Stderr, "Unknown argument %q\n", args)
		os.Exit(2)
	}

	os.Exit(0)
}
