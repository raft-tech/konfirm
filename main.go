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
	"context"
	_ "embed"
	"errors"
	"os"
	"os/signal"

	"github.com/raft-tech/konfirm/cmd"
	"github.com/raft-tech/konfirm/internal/cli"
)

var (
	//go:embed VERSION
	version string
)

func main() {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	konfirm := cmd.New()
	konfirm.Version = version
	if err := konfirm.ExecuteContext(ctx); err != nil {
		code := 1
		var xerr cli.ExitError
		if errors.As(err, &xerr) {
			code = xerr.ExitCode()
		}
		os.Exit(code)
	}
}
