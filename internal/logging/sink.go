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

package logging

import (
	"errors"
	"io"
	"net/url"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sinks = make(map[string]zap.Sink)

func init() {
	if e := zap.RegisterSink("konfirm", func(s *url.URL) (zap.Sink, error) {
		name := s.Path
		if strings.HasPrefix(name, "/") {
			name = name[1:]
		}
		if zs, ok := sinks[name]; ok {
			return zs, nil
		} else {
			return nil, errors.New("sink not found")
		}
	}); e != nil {
		panic(e)
	}
}

func RegisterSink(name string, writer io.Writer) {
	var zs zap.Sink
	if s, ok := writer.(zap.Sink); ok {
		zs = s
	} else {
		fake := &sink{
			WriteSyncer: zapcore.AddSync(writer),
		}
		if w, ok := writer.(io.WriteCloser); ok {
			fake.closer = w
		} else {
			fake.closer = nil
		}
		zs = fake
	}
	sinks[name] = zs
}

type sink struct {
	zapcore.WriteSyncer
	closer io.WriteCloser
}

func (s *sink) Close() error {
	var err error
	if w := s.closer; w != nil {
		err = w.Close()
	}
	return err
}
