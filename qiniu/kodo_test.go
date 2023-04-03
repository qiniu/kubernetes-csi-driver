package qiniu

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func getAccessKey() string {
	return os.Getenv("QINIU_ACCESS_KEY")
}
func getSecretKey() string {
	return os.Getenv("QINIU_SECRET_KEY")
}
func getUcUrl() *url.URL {
	ucUrl, err := url.Parse(strings.TrimSpace(os.Getenv("QINIU_TEST_UC_HOST")))
	if err != nil {
		panic(err)
	}
	return ucUrl
}

func getKodoClient() *KodoClient {
	return NewKodoClient(
		getAccessKey(),
		getSecretKey(),
		getUcUrl(),
		"", "",
	)
}

func TestKodoClient_CreateAndDeleteBucket(t *testing.T) {
	kodoClient := getKodoClient()
	err := kodoClient.CreateBucket(context.Background(), "csi-driver-test", "z0")
	assert.NoError(t, err)
	defer func() {
		err := kodoClient.DeleteBucket(context.Background(), "csi-driver-test")
		assert.NoError(t, err)
	}()
}

func TestKodoClient_GetRegions(t *testing.T) {
	kodoClient := getKodoClient()
	regions, err := kodoClient.GetRegions(context.Background())
	assert.NoError(t, err)
	assert.NotEmpty(t, regions)
}
