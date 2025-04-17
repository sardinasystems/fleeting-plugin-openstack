package openstackclient

import (
	"context"
	"os"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/testhelper"
	thclient "github.com/gophercloud/gophercloud/v2/testhelper/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetImageProperties(t *testing.T) {
	assert := assert.New(t)

	img, err := os.ReadFile("../../testdata/image_get.json")
	require.NoError(t, err)

	testhelper.SetupHTTP()
	defer testhelper.TeardownHTTP()

	testhelper.ServeFile(t, "", "", "application/json", string(img))

	client := &client{
		compute: thclient.ServiceClient(),
		image:   thclient.ServiceClient(),
	}

	ctx := context.TODO()
	props, err := client.GetImageProperties(ctx, "1da9661c-953e-424d-a1e5-834a8174b198")
	assert.NoError(err)
	if assert.NotNil(props) {
		assert.Equal("core", props.OSAdminUser)
	}

	t.Log(props)
}

func TestGetImageByName(t *testing.T) {
	assert := assert.New(t)

	img, err := os.ReadFile("../../testdata/image_list_one.json")
	require.NoError(t, err)

	testhelper.SetupHTTP()
	defer testhelper.TeardownHTTP()

	testhelper.ServeFile(t, "", "", "application/json", string(img))

	client := &client{
		compute: thclient.ServiceClient(),
		image:   thclient.ServiceClient(),
	}

	ctx := context.TODO()
	imageRef, props, err := client.GetImageByName(ctx, "flatcar_production_openstack_3815.2.5_amd64.raw")
	assert.NoError(err)
	assert.Equal("463074fa-f5cb-4601-b5da-5c45b9aa9981", imageRef)
	if assert.NotNil(props) {
		assert.Equal("core", props.OSAdminUser)
	}

	t.Log(props)
}

func TestGetImageByName_Many(t *testing.T) {
	assert := assert.New(t)

	img, err := os.ReadFile("../../testdata/image_list_many.json")
	require.NoError(t, err)

	testhelper.SetupHTTP()
	defer testhelper.TeardownHTTP()

	testhelper.ServeFile(t, "", "", "application/json", string(img))

	client := &client{
		compute: thclient.ServiceClient(),
		image:   thclient.ServiceClient(),
	}

	ctx := context.TODO()
	_, _, err = client.GetImageByName(ctx, "flatcar")
	assert.ErrorIs(err, gophercloud.ErrMultipleResourcesFound{Name: "flatcar", Count: 8, ResourceType: "image"})
}
