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
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func performBackup() error {
	// Handle log file rotation (before any output to log file so old one doesn't get trashed)

	logMessage(nil, "Rotating log files")
	if err := rotateLogFiles(); err != nil {
		return err
	}

	// Create output log file
	file, err := os.Create(filepath.Join(globalLogDir, cmdConfig+".log"))
	if err != nil {
		logError(nil, fmt.Sprint("Error: ", err))
		return err
	}
	logger := log.New(file, "", log.Ltime)

	startTime := time.Now()

	logMessage(logger, fmt.Sprint("Beginning backup on ", time.Now().Format("01-02-2006 15:04:05")))

	// Perform "duplicacy backup" if required
	if cmdBackup {
		if err := performDuplicacyBackup(logger); err != nil {
			return err
		}
	}

	// Perform "duplicacy prune" if required
	if cmdPrune {
		if err := performDuplicacyPrune(logger); err != nil {
			return err
		}
	}

	// Perform "duplicacy check" if required
	if cmdCheck {
		if err := performDuplicacyCheck(logger); err != nil {
			return err
		}
	}

	endTime := time.Now()

	logger.Println("######################################################################")
	logMessage(logger, fmt.Sprint("Operations completed in ", getTimeDiffString(startTime, endTime)))

	return nil
}

func performDuplicacyBackup(logger *log.Logger) error {
	// Handling when processing output from "duplicacy backup" command
	var backupEntry backupRevision
	var copyEntry copyRevision

	backupLogger := func(line string) {
		switch {
		// Files: 161318 total, 1666G bytes; 373 new, 15,951M bytes
		case strings.HasPrefix(line, "Files:"):
			logger.Println(line)
			logMessage(logger, fmt.Sprint("  ", line))

			// Save chunk data for inclusion into HTML portion of E-Mail message
			re := regexp.MustCompile(`.*: (\S+) total, (\S+) bytes; (\S+) new, (\S+) bytes`)
			elements := re.FindStringSubmatch(line)
			if len(elements) >= 4 {
				backupEntry.filesTotalCount = elements[1]
				backupEntry.filesTotalSize = elements[2]
				backupEntry.filesNewCount = elements[3]
				backupEntry.filesNewSize = elements[4]
			}

			// All chunks: 348444 total, 1668G bytes; 2415 new, 12,391M bytes, 12,255M bytes uploaded
		case strings.HasPrefix(line, "All chunks:"):
			logger.Println(line)
			logMessage(logger, fmt.Sprint("  ", line))

			// Save chunk data for inclusion into HTML portion of E-Mail message
			re := regexp.MustCompile(`.*: (\S+) total, (\S+) bytes; (\S+) new, (\S+) bytes, (\S+) bytes uploaded`)
			elements := re.FindStringSubmatch(line)
			if len(elements) >= 6 {
				backupEntry.chunkTotalCount = elements[1]
				backupEntry.chunkTotalSize = elements[2]
				backupEntry.chunkNewCount = elements[3]
				backupEntry.chunkNewSize = elements[4]
				backupEntry.chunkNewUploaded = elements[5]
			}

			// Try to catch and point out password problems within dupliacy
		case strings.HasSuffix(line, "Authorization failure"):
			logMessage(logger, "  Error: Duplicacy appears to be prompting for a password")

			logger.Println(line)
			logMessage(logger, fmt.Sprint("  ", line))

		default:
			logger.Println(line)
		}
	}

	copyLogger := func(line string) {
		switch {
		// Copy complete, 107 total chunks, 0 chunks copied, 107 skipped
		case strings.HasPrefix(line, "Copy complete, "):
			logger.Println(line)
			logMessage(logger, fmt.Sprint("  ", line))

			// Save chunk data for inclusion into HTML portion of E-Mail message
			re := regexp.MustCompile(`Copy complete, (\S+) total chunks, (\S+) chunks copied, (\S+) skipped`)
			elements := re.FindStringSubmatch(line)
			if len(elements) >= 4 {
				copyEntry.chunkTotalCount = elements[1]
				copyEntry.chunkCopyCount = elements[2]
				copyEntry.chunkSkipCount = elements[3]
			}
		default:
			logger.Println(line)
		}
	}

	// Perform backup/copy operations
	for _, backupInfo := range configFile.backupInfo {
		backupStartTime := time.Now()
		logger.Println("######################################################################")
		cmdArgs := []string{"backup", "-storage", backupInfo["name"], "-threads", backupInfo["threads"], "-stats"}
		vssFlags := ""
		if backupInfo["vss"] == "true" {
			cmdArgs = append(cmdArgs, "-vss")
			vssFlags = " -vss"
			if backupInfo["vssTimeout"] != "" {
				cmdArgs = append(cmdArgs, "-vss-timeout", backupInfo["vssTimeout"])
				vssFlags = fmt.Sprintf("%s -vss-timeout %s", vssFlags, backupInfo["vssTimeout"])
			}
		}

		logMessage(logger, fmt.Sprint("Backing up to storage ", backupInfo["name"],
			vssFlags, " with ", backupInfo["threads"], " threads"))

		if debugFlag {
			logMessage(logger, fmt.Sprint("Executing: ", duplicacyPath, cmdArgs))
		}
		err := Executor(duplicacyPath, cmdArgs, configFile.repoDir, backupLogger)
		if err != nil {
			logError(logger, fmt.Sprint("Error executing command: ", err))
			return err
		}
		backupDuration := getTimeDiffString(backupStartTime, time.Now())
		logMessage(logger, fmt.Sprint("  Duration: ", backupDuration))

		// Save data from backup for HTML table in E-Mail
		backupEntry.storage = backupInfo["name"]
		backupEntry.duration = backupDuration
		backupTable = append(backupTable, backupEntry)
	}

	if len(configFile.copyInfo) != 0 {
		for _, copyInfo := range configFile.copyInfo {
			copyStartTime := time.Now()
			logger.Println("######################################################################")
			cmdArgs := []string{"copy", "-threads", copyInfo["threads"],
				"-from", copyInfo["from"], "-to", copyInfo["to"]}
			logMessage(logger, fmt.Sprint("Copying from storage ", copyInfo["from"],
				" to storage ", copyInfo["to"], " with ", copyInfo["threads"], " threads"))
			if debugFlag {
				logMessage(logger, fmt.Sprint("Executing: ", duplicacyPath, cmdArgs))
			}
			err := Executor(duplicacyPath, cmdArgs, configFile.repoDir, copyLogger)
			if err != nil {
				logError(logger, fmt.Sprint("Error executing command: ", err))
				return err
			}
			copyDuration := getTimeDiffString(copyStartTime, time.Now())
			logMessage(logger, fmt.Sprint("  Duration: ", getTimeDiffString(copyStartTime, time.Now())))

			// Save data from backup for HTML table in E-Mail
			copyEntry.storageFrom = copyInfo["from"]
			copyEntry.storageTo = copyInfo["to"]
			copyEntry.duration = copyDuration
			copyTable = append(copyTable, copyEntry)
		}
	}

	return nil
}

func performDuplicacyPrune(logger *log.Logger) error {
	// Handling when processing output from generic "duplicacy" command
	anon := func(s string) { logger.Println(s) }

	// Perform prune operations
	for _, pruneInfo := range configFile.pruneInfo {
		logger.Println("######################################################################")
		cmdArgs := []string{"prune", "-all", "-storage", pruneInfo["storage"]}
		cmdArgs = append(cmdArgs, strings.Split(pruneInfo["keep"], " ")...)
		logMessage(logger, fmt.Sprint("Pruning storage ", pruneInfo["storage"]))
		if debugFlag {
			logMessage(logger, fmt.Sprint("Executing: ", duplicacyPath, cmdArgs))
		}
		err := Executor(duplicacyPath, cmdArgs, configFile.repoDir, anon)
		if err != nil {
			logError(logger, fmt.Sprint("Error executing command: ", err))
			return err
		}
	}

	return nil
}

func performDuplicacyCheck(logger *log.Logger) error {
	// Handling when processing output from generic "duplicacy" command
	anon := func(s string) { logger.Println(s) }

	// Perform check operations
	for _, checkInfo := range configFile.checkInfo {
		logger.Println("######################################################################")
		cmdArgs := []string{"check", "-storage", checkInfo["storage"]}
		if checkInfo["all"] == "true" {
			cmdArgs = append(cmdArgs, "-all")
		}
		logMessage(logger, fmt.Sprint("Checking storage ", checkInfo["storage"]))
		if debugFlag {
			logMessage(logger, fmt.Sprint("Executing: ", duplicacyPath, cmdArgs))
		}
		err := Executor(duplicacyPath, cmdArgs, configFile.repoDir, anon)
		if err != nil {
			logError(logger, fmt.Sprint("Error executing command: ", err))
			return err
		}
	}

	return nil
}
