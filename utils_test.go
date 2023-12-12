package fpoc

import (
	"encoding/json"
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

func TestExtCreateOpts(t *testing.T) {
	assert := assert.New(t)

	cfgJSON := `
	{
		"name": "gitlab-runner-%d",
		"description": "podman instance",
		"imageRef": "f2403879-6fbe-49a0-b71f-54b70039f32a",
		"flavorRef": "5",
		"key_name": "gitlab-autoscaler",
		"networks": [{"uuid": "c487d046-80ad-4da0-8b98-4a48ad3c257a"}],
		"security_groups": ["allow_gitlab_runner"],
		"os:scheduler_hints": {"group": "a5b557be-b7f0-4cb3-8f7c-6b5092f29c2c"},
		"tags": ["podman", "CI"],
		"user_data": "#!cloud-config\npackage_update: true\npackage_upgrade: true\n",
		"metadata": {"foo": "bar"}
	}
	`

	expected := `{"server":{"description":"podman instance","flavorRef":"5","imageRef":"f2403879-6fbe-49a0-b71f-54b70039f32a","key_name":"gitlab-autoscaler","metadata":{"foo":"bar"},"name":"gitlab-runner-%d","networks":[{"uuid":"c487d046-80ad-4da0-8b98-4a48ad3c257a"}],"os:scheduler_hints":{"group":"a5b557be-b7f0-4cb3-8f7c-6b5092f29c2c"},"security_groups":[{"name":"allow_gitlab_runner"}],"tags":["podman","CI"],"user_data":"IyFjbG91ZC1jb25maWcKcGFja2FnZV91cGRhdGU6IHRydWUKcGFja2FnZV91cGdyYWRlOiB0cnVlCg=="}}`

	cfg := new(ExtCreateOpts)
	err := json.Unmarshal([]byte(cfgJSON), cfg)
	assert.NoError(err)

	omap, err := cfg.ToServerCreateMap()
	assert.NoError(err)
	assert.NotNil(omap)

	req, err := json.Marshal(omap)
	assert.NoError(err)
	assert.Equal(expected, string(req))

	//t.Log(omap)
	//t.Log(string(req))
}