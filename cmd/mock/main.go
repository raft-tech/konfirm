/*
 Copyright 2022 Raft, LLC

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"os"
)

func main() {
	exitCode := 0
	argv := os.Args
	var message string
	switch argc := len(argv); true {
	case argc == 2:
		if a := argv[1]; a == "--fail" {
			exitCode = 1
		} else {
			message = a
		}
	case argc == 3:
		if argv[1] == "--fail" {
			exitCode = 1
		}
		message = argv[2]
	}
	if message != "" {
		_ = os.WriteFile("/dev/termination-log", []byte(message), 0644)
	}
	os.Exit(exitCode)
}
