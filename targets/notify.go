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
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// Report is what a run delivers through its notifiers.
type Report struct {
	Subject        string
	TextBody       string
	HTMLBody       string
	AttachmentName string
	AttachmentType string // MIME type, e.g. "text/csv"
	Attachment     []byte
}

type Notifier func(ctx context.Context, r Report)

// NotifyConfig selects the notifiers; an empty field disables its notifier.
type NotifyConfig struct {
	ReportDir   string
	AlertCmd    string
	EmailFrom   string
	EmailTo     string // comma-separated
	EmailRegion string // AWS region for SES; the default chain's when empty
}

func (c NotifyConfig) Notifiers() []Notifier {
	var out []Notifier
	if c.ReportDir != "" {
		out = append(out, FileNotifier(c.ReportDir))
	}
	if c.AlertCmd != "" {
		out = append(out, CmdNotifier(c.AlertCmd))
	}
	if c.EmailFrom != "" && c.EmailTo != "" {
		out = append(out, SESNotifier(c.EmailFrom, c.EmailTo, c.EmailRegion))
	}
	return out
}

// FileNotifier writes report-latest.txt, report-latest.html and the
// attachment into dir.
func FileNotifier(dir string) Notifier {
	return func(_ context.Context, r Report) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("report-dir: %v", err)
			return
		}
		files := map[string][]byte{
			"report-latest.txt":  []byte(r.TextBody),
			"report-latest.html": []byte(r.HTMLBody),
		}
		if r.AttachmentName != "" {
			files[r.AttachmentName] = r.Attachment
		}
		for name, data := range files {
			if len(data) == 0 {
				continue
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				log.Printf("report-dir write %s: %v", name, err)
			}
		}
		log.Printf("report written to %s", dir)
	}
}

// CmdNotifier runs cmdLine under sh with the text report on stdin and the
// subject in ALERT_SUBJECT.
func CmdNotifier(cmdLine string) Notifier {
	return func(ctx context.Context, r Report) {
		cmd := exec.CommandContext(ctx, "sh", "-c", cmdLine)
		cmd.Env = append(cmd.Environ(), "ALERT_SUBJECT="+r.Subject)
		cmd.Stdin = strings.NewReader(r.TextBody)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("alert-cmd failed: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
}

// SESNotifier emails the HTML report with its attachment through SES.
func SESNotifier(from, toCSV, region string) Notifier {
	var to []string
	for _, p := range strings.Split(toCSV, ",") {
		if v := strings.TrimSpace(p); v != "" {
			to = append(to, v)
		}
	}
	return func(ctx context.Context, r Report) {
		var opts []func(*config.LoadOptions) error
		if region != "" {
			opts = append(opts, config.WithRegion(region))
		}
		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			log.Printf("ses config: %v", err)
			return
		}
		raw := BuildRawEmail(from, strings.Join(to, ", "), r)
		_, err = sesv2.NewFromConfig(cfg).SendEmail(ctx, &sesv2.SendEmailInput{
			FromEmailAddress: aws.String(from),
			Destination:      &types.Destination{ToAddresses: to},
			Content:          &types.EmailContent{Raw: &types.RawMessage{Data: raw}},
		})
		if err != nil {
			log.Printf("ses send: %v", err)
			return
		}
		log.Printf("report emailed to %s", strings.Join(to, ", "))
	}
}

func BuildRawEmail(from, to string, r Report) []byte {
	const boundary = "mixed-boundary-dumbodb-compare"
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", r.Subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(r.HTMLBody)
	b.WriteString("\r\n")

	if len(r.Attachment) > 0 {
		contentType := r.AttachmentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; name=\"%s\"\r\n", contentType, r.AttachmentName)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=\"%s\"\r\n\r\n", r.AttachmentName)
		encoded := base64.StdEncoding.EncodeToString(r.Attachment)
		for i := 0; i < len(encoded); i += 76 {
			b.WriteString(encoded[i:min(i+76, len(encoded))])
			b.WriteString("\r\n")
		}
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}
