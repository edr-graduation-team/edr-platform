package detection

import (
	"encoding/base64"
	"testing"

	"github.com/edr-platform/sigma-engine/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestTryDecodePowerShellEncodedCommand_UTF16LE(t *testing.T) {
	// "Write-Host hi" encoded as UTF-16LE then base64 (PowerShell style).
	utf16le := []byte{
		'W', 0, 'r', 0, 'i', 0, 't', 0, 'e', 0, '-', 0, 'H', 0, 'o', 0, 's', 0, 't', 0, ' ', 0, 'h', 0, 'i', 0,
	}
	b64 := base64.StdEncoding.EncodeToString(utf16le)

	ev, err := domain.NewLogEvent(map[string]interface{}{
		"process": map[string]interface{}{
			"command_line": "powershell.exe -NoP -Enc " + b64,
		},
	})
	assert.NoError(t, err)

	p := tryDecodePowerShellEncodedCommand(ev)
	assert.NotNil(t, p)
	assert.Equal(t, "powershell_encodedcommand", p.Kind)
	assert.Equal(t, "utf-16le", p.Charset)
	assert.Equal(t, b64, p.Encoded)
	assert.Contains(t, p.Decoded, "Write-Host hi")
}

func TestEnrichMatchedFieldsWithDecodedPayload_AddsFields(t *testing.T) {
	utf16le := []byte{'i', 0, 'p', 0, 'c', 0, 'o', 0, 'n', 0, 'f', 0, 'i', 0, 'g', 0}
	b64 := base64.StdEncoding.EncodeToString(utf16le)

	ev, err := domain.NewLogEvent(map[string]interface{}{
		"CommandLine": "powershell -encodedcommand \"" + b64 + "\"",
	})
	assert.NoError(t, err)

	matched := map[string]interface{}{}
	enrichMatchedFieldsWithDecodedPayload(ev, matched)

	assert.Equal(t, "powershell_encodedcommand", matched["decoded_command_kind"])
	assert.Equal(t, "utf-16le", matched["decoded_command_charset"])
	assert.Equal(t, b64, matched["decoded_command_base64"])
	assert.Contains(t, matched["decoded_command"].(string), "ipconfig")
}
