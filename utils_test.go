package fpoc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsCloudInitFinished(t *testing.T) {
	testCases := []struct {
		name     string
		readLen  int
		expected bool
	}{
		{"token-not-fond", 4096, false},
		{"finished", 102400, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := os.ReadFile("testdata/console_out.txt")
			require.NoError(t, err)

			var log string
			if len(buf) >= tc.readLen {
				log = string(buf[0:tc.readLen])
			} else {
				log = string(buf)
			}

			obtained := IsCloudInitFinished(log)
			assert.Equal(t, tc.expected, obtained)
		})
	}
}
