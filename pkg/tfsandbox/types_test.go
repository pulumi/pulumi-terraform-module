package tfsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsLocalPath(t *testing.T) {
	assert.True(t, TFModuleSource("../local-module").IsLocalPath())
	assert.True(t, TFModuleSource("./local-module").IsLocalPath())
	assert.False(t, TFModuleSource("hashicorp/consul/aws").IsLocalPath())
}
