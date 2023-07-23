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
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"strconv"
)

type Logger struct {
	logr.Logger
	debug logr.Logger
	trace logr.Logger
}

func NewLogger(from logr.Logger) *Logger {
	return &Logger{
		Logger: from,
		debug:  from.V(1),
		trace:  from.V(2),
	}
}

func (l *Logger) DebugLogger() logr.Logger {
	return l.debug
}

func (l *Logger) Debug(msg string, keysAndValues ...interface{}) {
	l.debug.Info(msg, keysAndValues...)
}

func (l *Logger) Trace(msg string, keysAndValues ...interface{}) {
	l.trace.Info(msg, keysAndValues...)
}

func (l *Logger) WithValues(keysAndValues ...interface{}) *Logger {
	return NewLogger(l.Logger.WithValues(keysAndValues...))
}

func FromContext(ctx context.Context, keysAndValues ...interface{}) *Logger {
	return NewLogger(logr.FromContextOrDiscard(ctx).WithValues(keysAndValues...))
}

func FromContextWithName(ctx context.Context, name string, keysAndValues ...interface{}) *Logger {
	return NewLogger(logr.FromContextOrDiscard(ctx).WithName(name).WithValues(keysAndValues...))
}

func EncoderLevel(enc zapcore.LevelEncoder) zap.Opts {
	return func(o *zap.Options) {
		o.EncoderConfigOptions = append(o.EncoderConfigOptions, EncoderLevelConfig(enc))
	}
}

func EncoderLevelConfig(enc zapcore.LevelEncoder) zap.EncoderConfigOption {
	return func(c *zapcore.EncoderConfig) {
		c.EncodeLevel = enc
	}
}

func LowercaseLevelEncoder(level zapcore.Level, encoder zapcore.PrimitiveArrayEncoder) {
	switch {
	case level < zapcore.DebugLevel:
		str := "trace"
		if l := int(zapcore.DebugLevel - level); l > 1 {
			str += strconv.Itoa(l)
		}
		encoder.AppendString(str)
	default:
		encoder.AppendString(level.String())
	}
}

func CapitalLevelEncoder(level zapcore.Level, encoder zapcore.PrimitiveArrayEncoder) {
	switch {
	case level < zapcore.DebugLevel:
		str := "TRACE"
		if l := int(zapcore.DebugLevel - level); l > 1 {
			str += strconv.Itoa(l)
		}
		encoder.AppendString(str)
	default:
		encoder.AppendString(level.CapitalString())
	}
}
