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

package targets

import (
	"fmt"
	"syscall"
)

const (
	tmpfsMagic = 0x01021994
	ramfsMagic = 0x858458f6
)

// RequireDiskBacked fails when dir is on a RAM filesystem.
func RequireDiskBacked(dir string) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return fmt.Errorf("statfs %s: %w", dir, err)
	}
	switch uint32(st.Type) {
	case tmpfsMagic, ramfsMagic:
		return fmt.Errorf("%s is on a RAM filesystem, so syncs never reach disk; set -work-dir to a disk-backed directory", dir)
	}
	return nil
}
