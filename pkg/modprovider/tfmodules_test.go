package modprovider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractModuleContentWorks(t *testing.T) {
	awsVpc, err := extractModuleContent("terraform-aws-modules/vpc/aws", "5.18.1")
	assert.NoError(t, err, "failed to infer module schema for aws vpc module")
	assert.NotNil(t, awsVpc, "inferred module schema for aws vpc module is nil")
}

func TestInferringModuleSchemaWorks(t *testing.T) {
	awsVpcSchema, err := InferModuleSchema("terraform-aws-modules/vpc/aws", "5.18.1")
	assert.NoError(t, err, "failed to infer module schema for aws vpc module")
	assert.NotNil(t, awsVpcSchema, "inferred module schema for aws vpc module is nil")
}
