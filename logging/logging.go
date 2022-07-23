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

package logging

import (
	"context"
	"github.com/go-logr/logr"
)

const (
	INFO = iota
	DEBUG
	TRACE
)

type Logger struct {
	logr.Logger
	debug logr.Logger
	trace logr.Logger
}

func NewLogger(from logr.Logger) *Logger {
	logger := &Logger{
		Logger: from,
	}
	logger.debug = logger.Logger.V(DEBUG)
	logger.trace = logger.Logger.V(TRACE)
	return logger
}

func (l *Logger) Debug() logr.Logger {
	return l.debug
}

func (l *Logger) Trace() logr.Logger {
	return l.trace
}

func (l *Logger) WithValues(keysAndValues ...interface{}) *Logger {
	return NewLogger(l.Logger.WithValues(keysAndValues))
}

func FromContext(ctx context.Context, keysAndValues ...interface{}) *Logger {
	return NewLogger(logr.FromContextOrDiscard(ctx))
}
