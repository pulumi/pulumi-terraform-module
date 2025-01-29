package tfsandbox

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTofuInit(t *testing.T) {
	tofu, err := NewTofu(context.Background(), "test")
	assert.NoError(t, err)
	tofu.WorkingDir()

	var res bytes.Buffer
	err = tofu.tf.InitJSON(context.Background(), &res)
	assert.NoError(t, err)
	t.Logf("Output: %s", res.String())

	assert.NoError(t, err)
	assert.Contains(t, res.String(), "OpenTofu initialized in an empty directory")
}
