/*
 Copyright 2024 Raft, LLC

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

package utils

import (
	"errors"
	"fmt"
)

func WrapError(err error, msg string) error {
	return &WrappedError{
		error:   errors.New(msg),
		wrapped: err,
	}
}

func WrapfError(err error, pattern string, args ...any) error {
	return WrapError(err, fmt.Sprintf(pattern, args...))
}

type WrappedError struct {
	error
	wrapped error
}

func (e *WrappedError) Unwrap() error {
	return e.wrapped
}

func UnwrapAndJoin(err error) error {
	var errs []error
	errs = append(errs, err)
	unwrapAndJoin(err, &errs)
	return errors.Join(errs...)
}

func unwrapAndJoin(err error, to *[]error) {
	if e := errors.Unwrap(err); e != nil {
		*to = append(*to, e)
		unwrapAndJoin(e, to)
	}
}