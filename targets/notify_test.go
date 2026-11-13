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
	"bytes"
	"encoding/base64"
	"testing"
)

func TestRawEmailAttachesCSV(t *testing.T) {
	r := Report{Subject: "s", HTMLBody: "<p>x</p>", AttachmentName: "run.csv", AttachmentType: "text/csv", Attachment: []byte("a,b\n1,2\n")}
	raw := BuildRawEmail("from@example.com", "to@example.com", r)
	for _, want := range []string{
		"multipart/mixed",
		`Content-Type: text/csv; name="run.csv"`,
		`Content-Disposition: attachment; filename="run.csv"`,
		base64.StdEncoding.EncodeToString(r.Attachment),
	} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("email lacks %q:\n%s", want, raw)
		}
	}
}
