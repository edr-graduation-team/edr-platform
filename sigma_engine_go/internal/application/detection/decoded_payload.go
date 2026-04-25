package detection

import (
	"encoding/base64"
	"regexp"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/edr-platform/sigma-engine/internal/domain"
)

type decodedPayload struct {
	Encoded string
	Decoded string
	Kind    string // e.g. "powershell_encodedcommand"
	Charset string // e.g. "utf-16le" or "utf-8"
}

var (
	// PowerShell encoded command variants: -enc, -encodedcommand, /enc, /encodedcommand
	// Capture the next token which may be quoted.
	rePSEncoded = regexp.MustCompile(`(?i)(?:^|\s)(?:-|/)(?:enc|encodedcommand)\s+(?:"([^"]+)"|'([^']+)'|([A-Za-z0-9+/_=-]{10,}))`)
)

func enrichMatchedFieldsWithDecodedPayload(event *domain.LogEvent, matchedFields map[string]interface{}) {
	if event == nil || matchedFields == nil {
		return
	}

	// If already present, do not overwrite (lets upstream enrichment win).
	if _, ok := matchedFields["decoded_command"]; ok {
		return
	}

	if p := tryDecodePowerShellEncodedCommand(event); p != nil && p.Decoded != "" {
		matchedFields["decoded_command"] = p.Decoded
		matchedFields["decoded_command_kind"] = p.Kind
		matchedFields["decoded_command_charset"] = p.Charset
		matchedFields["decoded_command_base64"] = p.Encoded
	}
}

func tryDecodePowerShellEncodedCommand(event *domain.LogEvent) *decodedPayload {
	cmdLine := firstNonEmpty(
		event.GetStringField("process.command_line"),
		event.GetStringField("data.command_line"),
		event.GetStringField("CommandLine"),
		event.GetStringField("command_line"),
	)
	if strings.TrimSpace(cmdLine) == "" {
		return nil
	}

	m := rePSEncoded.FindStringSubmatch(cmdLine)
	if len(m) == 0 {
		return nil
	}

	encoded := firstNonEmpty(m[1], m[2], m[3])
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil
	}

	decodedBytes, charset, ok := decodePowerShellBase64(encoded)
	if !ok {
		return nil
	}

	decoded := strings.TrimSpace(string(decodedBytes))
	if decoded == "" {
		return nil
	}

	return &decodedPayload{
		Encoded: encoded,
		Decoded: decoded,
		Kind:    "powershell_encodedcommand",
		Charset: charset,
	}
}

func decodePowerShellBase64(s string) ([]byte, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", false
	}

	// PowerShell often uses standard base64, but accept a few variants defensively.
	decoders := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}

	var b []byte
	var err error
	for _, dec := range decoders {
		b, err = dec(s)
		if err == nil && len(b) > 0 {
			break
		}
	}
	if err != nil || len(b) == 0 {
		return nil, "", false
	}

	// Typical PowerShell -EncodedCommand is UTF-16LE.
	if looksLikeUTF16LE(b) {
		u16 := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			u16 = append(u16, uint16(b[i])|uint16(b[i+1])<<8)
		}
		runes := utf16.Decode(u16)
		out := []byte(string(runes))
		return out, "utf-16le", true
	}

	// Otherwise, treat as UTF-8 if it validates, else return bytes as-is.
	if utf8.Valid(b) {
		return b, "utf-8", true
	}
	return b, "binary", true
}

func looksLikeUTF16LE(b []byte) bool {
	if len(b) < 6 || len(b)%2 != 0 {
		return false
	}
	// Heuristic: lots of 0x00 in the high byte of ASCII range text.
	zeros := 0
	samples := 0
	for i := 1; i < len(b) && samples < 64; i += 2 {
		samples++
		if b[i] == 0x00 {
			zeros++
		}
	}
	return samples >= 6 && float64(zeros)/float64(samples) >= 0.6
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
