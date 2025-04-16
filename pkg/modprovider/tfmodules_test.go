// Copyright 2016-2025, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modprovider

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func TestExtractModuleContentWorks(t *testing.T) {
	ctx := context.Background()
	srv := newTestAuxProviderServer(t)
	awsVpc, err := extractModuleContent(ctx, "terraform-aws-modules/vpc/aws", "5.18.1",
		tfsandbox.DiscardLogger, srv)
	assert.NoError(t, err, "failed to infer module schema for aws vpc module")
	assert.NotNil(t, awsVpc, "inferred module schema for aws vpc module is nil")
}

func TestInferringModuleSchemaWorks(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("terraform-aws-modules")
	srv := newTestAuxProviderServer(t)
	awsVpcSchema, err := InferModuleSchema(ctx, packageName, "terraform-aws-modules/vpc/aws", "5.19.0", srv)
	assert.NoError(t, err, "failed to infer module schema for aws vpc module")
	assert.NotNil(t, awsVpcSchema, "inferred module schema for aws vpc module is nil")
	// verify a sample of the inputs with different inferred types
	expectedSampleInputs := map[string]*schema.PropertySpec{
		"cidr": {
			Description: "(Optional) The IPv4 CIDR block for the VPC. CIDR can be" +
				" explicitly set or it can be derived from IPAM using `ipv4_netmask_length` & `ipv4_ipam_pool_id`",
			Secret:   false,
			TypeSpec: stringType,
		},
		"create_database_subnet_route_table": {
			Description: "Controls if separate route table for database should be created",
			Secret:      false,
			TypeSpec:    boolType,
		},
		"azs": {
			Description: "A list of availability zones names or ids in the region",
			Secret:      false,
			TypeSpec:    arrayType(stringType),
		},
		"customer_gateway_tags": {
			Description: "Additional tags for the Customer Gateway",
			Secret:      false,
			TypeSpec:    mapType(stringType),
		},
		"vpc_block_public_access_exclusions": {
			Description: "A map of VPC block public access exclusions",
			Secret:      false,
			TypeSpec:    mapType(anyType),
		},
		"customer_gateways": {
			Description: "Maps of Customer Gateway's attributes (BGP ASN and Gateway's Internet-routable external IP address)",
			Secret:      false,
			TypeSpec:    mapType(mapType(anyType)),
		},
		"database_inbound_acl_rules": {
			Description: "Database subnets inbound network ACL rules",
			Secret:      false,
			TypeSpec:    arrayType(mapType(stringType)),
		},
	}

	for name, expected := range expectedSampleInputs {
		actual, ok := awsVpcSchema.Inputs[name]
		assert.True(t, ok, "input %s is missing from the schema", name)
		assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
		assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
		assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
	}

	// verify a sample of the outputs with different inferred types
	expectedSampleOutputs := map[string]*schema.PropertySpec{
		"vpc_id": {
			Description: "The ID of the VPC",
			Secret:      false,
			TypeSpec:    stringType,
		},
		// from expression compact(aws_vpc_ipv4_cidr_block_association.this[*].cidr_block)
		"vpc_secondary_cidr_blocks": {
			Description: "List of secondary CIDR blocks of the VPC",
			Secret:      false,
			TypeSpec:    arrayType(stringType),
		},
		// from expression aws_subnet.public[*].id
		"public_subnets": {
			Description: "List of IDs of public subnets",
			Secret:      false,
			TypeSpec:    arrayType(stringType),
		},
		// from conditional expression
		// length(aws_route_table.database[*].id) > 0
		//     ? aws_route_table.database[*].id
		//     : aws_route_table.private[*].id
		"database_route_table_ids": {
			Description: "List of IDs of database route tables",
			Secret:      false,
			TypeSpec:    arrayType(stringType),
		},
		// from expression [for k, v in aws_customer_gateway.this : v.id]
		"cgw_ids": {
			Description: "List of IDs of Customer Gateway",
			Secret:      false,
			TypeSpec:    arrayType(stringType),
		},
		// from expression var.flow_log_destination_type
		// which is a variable defined in the module
		// we take the same type as that variable
		"vpc_flow_log_destination_type": {
			Description: "The type of the destination for VPC Flow Logs",
			Secret:      false,
			TypeSpec:    awsVpcSchema.Inputs["flow_log_destination_type"].TypeSpec,
		},
	}

	for name, expected := range expectedSampleOutputs {
		actual, ok := awsVpcSchema.Outputs[name]
		assert.True(t, ok, "output %s is missing from the schema", name)
		assert.Equal(t, expected.Description, actual.Description, "output %s description is incorrect", name)
		assert.Equal(t, expected.Secret, actual.Secret, "output %s secret is incorrect", name)
		assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "output %s type is incorrect", name)
	}

	expectedConfigVariables := map[string]schema.PropertySpec{
		"aws": {
			TypeSpec:    mapType(anyType),
			Description: "provider configuration for aws",
		},
	}
	assert.Equal(t,
		expectedConfigVariables,
		awsVpcSchema.ProvidersConfig.Variables,
		"required provider variables are incorrect")
}

func TestInferModuleSchemaFromGitHubSource(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("demoWebsite")
	version := TFModuleVersion("") // GitHub-sourced modules don't take a version
	srv := newTestAuxProviderServer(t)
	demoSchema, err := InferModuleSchema(ctx, packageName, "github.com/yemisprojects/s3_website_module_demo", version, srv)
	assert.NoError(t, err, "failed to infer module schema for github module")
	assert.NotNil(t, demoSchema, "inferred module schema for aws vpc module is nil")
	// verify a sample of the inputs with different inferred types
	expectedSampleInputs := map[string]*schema.PropertySpec{
		"bucket_name": {
			Description: "Name of S3 bucket for the website",
			Secret:      false,
			TypeSpec:    stringType,
		},
		"environment": {
			Description: "Environment bucket resides in",
			Secret:      false,
			TypeSpec:    stringType,
		},
	}

	for name, expected := range expectedSampleInputs {
		actual, ok := demoSchema.Inputs[name]
		assert.True(t, ok, "input %s is missing from the schema", name)
		assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
		assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
		assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
	}
}

func TestInferModuleSchemaFromGitHubSourceWithSubModule(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("consulCluster")
	version := TFModuleVersion("") // GitHub-sourced modules don't take a version
	consulClusterSchema, err := InferModuleSchema(ctx,
		packageName,
		"github.com/hashicorp/terraform-aws-consul//modules/consul-cluster",
		version,
		newTestAuxProviderServer(t),
	)
	assert.NoError(t, err, "failed to infer module schema for github submodule")
	assert.NotNil(t, consulClusterSchema, "inferred module schema for aws consul cluster submodule is nil")
	// verify a sample of the inputs with different inferred types
	expectedSampleInputs := map[string]*schema.PropertySpec{
		"ami_id": {
			Description: "The ID of the AMI to run in this cluster. " +
				"Should be an AMI that had Consul installed and configured by the install-consul module.",
			Secret:   false,
			TypeSpec: stringType,
		},
		"spot_price": {
			Description: "The maximum hourly price to pay for EC2 Spot Instances.",
			Secret:      false,
			TypeSpec:    numberType,
		},
	}

	for name, expected := range expectedSampleInputs {
		actual, ok := consulClusterSchema.Inputs[name]
		assert.True(t, ok, "input %s is missing from the schema", name)
		assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
		assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
		assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
	}
}

func TestResolveModuleSources(t *testing.T) {
	t.Run("local path-based module source", func(t *testing.T) {
		ctx := context.Background()
		src := filepath.Join("..", "..", "tests", "testdata", "modules", "randmod")
		p, err := filepath.Abs(src)
		require.NoError(t, err)
		d, err := resolveModuleSources(ctx, TFModuleSource(p), "", tfsandbox.DiscardLogger,
			newTestAuxProviderServer(t))
		require.NoError(t, err)

		bytes, err := os.ReadFile(filepath.Join(d, "variables.tf"))
		require.NoError(t, err)

		t.Logf("variables.tf: %s", bytes)

		assert.Contains(t, string(bytes), "maxlen")
	})

	// This test will hit the network to download a well-known module from a registry.
	t.Run("registry module source", func(t *testing.T) {
		ctx := context.Background()
		s := TFModuleSource("terraform-aws-modules/s3-bucket/aws")
		v := TFModuleVersion("4.5.0")
		d, err := resolveModuleSources(ctx, s, v, tfsandbox.DiscardLogger,
			newTestAuxProviderServer(t))
		require.NoError(t, err)

		bytes, err := os.ReadFile(filepath.Join(d, "variables.tf"))
		require.NoError(t, err)

		t.Logf("variables.tf: %s", bytes)
		assert.Contains(t, string(bytes), "putin_khuylo")
	})

	// Make a network call to resolve the source for a remote module source on GitHub.
	t.Run("remote module source github", func(t *testing.T) {
		ctx := context.Background()
		moduleSource := TFModuleSource("github.com/yemisprojects/s3_website_module_demo")
		workingDirectory, err := resolveModuleSources(ctx, moduleSource, "", tfsandbox.DiscardLogger,
			newTestAuxProviderServer(t))
		require.NoError(t, err)

		bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
		require.NoError(t, err)

		t.Logf("variables.tf: %s", bytes)
		assert.Contains(t, string(bytes), "index_document")
	})

	t.Run("remote module source with version in source path", func(t *testing.T) {
		ctx := context.Background()
		moduleSource := TFModuleSource("github.com/yemisprojects/s3_website_module_demo?ref=v0.0.1")
		workingDirectory, err := resolveModuleSources(ctx, moduleSource, "", tfsandbox.DiscardLogger,
			newTestAuxProviderServer(t))
		require.NoError(t, err)

		bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
		require.NoError(t, err)

		t.Logf("variables.tf: %s", bytes)
		assert.Contains(t, string(bytes), "index_document")
	})

	t.Run("remote module source with git path prefix", func(t *testing.T) {
		ctx := context.Background()
		moduleSource := TFModuleSource("git::github.com/yemisprojects/s3_website_module_demo?ref=v0.0.1")
		workingDirectory, err := resolveModuleSources(ctx, moduleSource, "", tfsandbox.DiscardLogger,
			newTestAuxProviderServer(t))
		require.NoError(t, err)

		bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
		require.NoError(t, err)

		t.Logf("variables.tf: %s", bytes)
		assert.Contains(t, string(bytes), "index_document")
	})
}
