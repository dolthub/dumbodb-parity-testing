// Copyright 2026 Dolthub, Inc.
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

package concurrency_test

import (
	"context"

	"github.com/dolthub/dumbodb-parity-testing/concurrency"
)

type externalTarget struct{}

func (externalTarget) Ping(context.Context) error {
	return nil
}

func (externalTarget) Identity(context.Context) (concurrency.ServerInfo, error) {
	return concurrency.ServerInfo{Product: "external", Version: "1"}, nil
}

func (externalTarget) Collection(string, string) concurrency.Collection {
	return nil
}

func (externalTarget) DropDatabase(context.Context, string) error {
	return nil
}

func (externalTarget) Disconnect(context.Context) error {
	return nil
}

var _ concurrency.Target = externalTarget{}
