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

package controllers

import (
	"errors"
	"strings"
)

type ErrorList struct {
	errors []error
}

func (e *ErrorList) Append(err error) {
	e.errors = append(e.errors, err)
}

func (e *ErrorList) HasError() bool {
	return len(e.errors) > 0
}

func (e *ErrorList) Error() error {
	if count := len(e.errors); count > 0 {
		err := strings.Builder{}
		err.WriteString(e.errors[0].Error())
		for i := 1; i < count; i++ {
			err.WriteString(e.errors[i].Error())
			err.WriteRune('\n')
		}
		return errors.New(err.String())
	} else {
		return nil
	}
}
