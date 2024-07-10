package fpoc

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud/v2/testhelper"
	tc "github.com/gophercloud/gophercloud/v2/testhelper/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsCloudInitFinished(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		readLen  int
		expected bool
	}{
		{"token-not-fond-1", "testdata/console_out.txt", 4096, false},
		{"finished-1", "testdata/console_out.txt", 102400, true},
		{"token-not-fond-2", "testdata/console_ubuntu2204.txt", 4096, false},
		{"finished-2", "testdata/console_ubuntu2204.txt", 102400, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := os.ReadFile(tc.file)
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

func TestIsIgnitionFinished(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		readLen  int
		expected bool
	}{
		{"token-not-fond-1", "testdata/console_flatcar.txt", 4096, false},
		{"finished-1", "testdata/console_flatcar.txt", 102400, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf, err := os.ReadFile(tc.file)
			require.NoError(t, err)

			var log string
			if len(buf) >= tc.readLen {
				log = string(buf[0:tc.readLen])
			} else {
				log = string(buf)
			}

			obtained := IsIgnitionFinished(log)
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
		"scheduler_hints": {"group": "a5b557be-b7f0-4cb3-8f7c-6b5092f29c2c"},
		"tags": ["podman", "CI"],
		"user_data": "#!cloud-config\npackage_update: true\npackage_upgrade: true\n",
		"metadata": {"foo": "bar"}
	}
	`

	expected := `{"server":{"description":"podman instance","flavorRef":"5","imageRef":"f2403879-6fbe-49a0-b71f-54b70039f32a","key_name":"gitlab-autoscaler","metadata":{"foo":"bar"},"name":"gitlab-runner-%d","networks":[{"uuid":"c487d046-80ad-4da0-8b98-4a48ad3c257a"}],"security_groups":[{"name":"allow_gitlab_runner"}],"tags":["podman","CI"],"user_data":"IyFjbG91ZC1jb25maWcKcGFja2FnZV91cGRhdGU6IHRydWUKcGFja2FnZV91cGdyYWRlOiB0cnVlCg=="}}`

	cfg := new(ExtCreateOpts)
	err := json.Unmarshal([]byte(cfgJSON), cfg)
	assert.NoError(err)

	assert.Equal("a5b557be-b7f0-4cb3-8f7c-6b5092f29c2c", cfg.SchedulerHints.Group)

	omap, err := cfg.ToServerCreateMap()
	assert.NoError(err)
	assert.NotNil(omap)

	req, err := json.Marshal(omap)
	assert.NoError(err)
	assert.Equal(expected, string(req))

	//t.Log(omap)
	//t.Log(string(req))
}

func TestGetImageProperties(t *testing.T) {
	assert := assert.New(t)

	img, err := os.ReadFile("./testdata/image_get.json")
	require.NoError(t, err)

	testhelper.SetupHTTP()
	defer testhelper.TeardownHTTP()

	testhelper.ServeFile(t, "", "", "application/json", string(img))

	ctx := context.TODO()
	imgCli := tc.ServiceClient()

	props, err := GetImageProperties(ctx, imgCli, "1da9661c-953e-424d-a1e5-834a8174b198")
	assert.NoError(err)
	if assert.NotNil(props) {
		assert.Equal("core", props.OSAdminUser)
	}

	t.Log(props)
}
