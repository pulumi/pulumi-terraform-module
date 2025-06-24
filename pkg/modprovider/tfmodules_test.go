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
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

func newTestRuntime(t *testing.T, executor string) *tfsandbox.ModuleRuntime {
	srv := newTestAuxProviderServer(t)

	tofu, err := tfsandbox.PickModuleRuntime(context.Background(), tfsandbox.DiscardLogger, nil, srv, executor)
	require.NoError(t, err, "failed to pick module runtime")
	return tofu
}

func TestExtractModuleContentWorks(t *testing.T) {
	executors := getExecutorsFromEnv()

	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			ctx := context.Background()
			logger := newTestLogger(t)
			source := TFModuleSource("terraform-aws-modules/vpc/aws")
			version := TFModuleVersion("5.18.1")
			tf := newTestRuntime(t, executor)
			awsVpc, err := extractModuleContent(ctx, tf, source, version, logger)
			assert.NoError(t, err, "failed to infer module schema for aws vpc module")
			assert.NotNil(t, awsVpc, "inferred module schema for aws vpc module is nil")
		})
	}
}

func getExecutorsFromEnv() []string {
	executor := os.Getenv("PULUMI_TERRAFORM_MODULE_EXECUTOR")
	if executor != "" {
		return []string{executor}
	}
	return []string{"opentofu", "terraform"}
}

func TestInferringModuleSchemaWorks(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("terraform-aws-modules")
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			source := TFModuleSource("terraform-aws-modules/vpc/aws")
			version := TFModuleVersion("5.19.0")
			tf := newTestRuntime(t, executor)
			awsVpcSchema, err := InferModuleSchema(ctx, tf, packageName, source, version)
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
				actual, ok := awsVpcSchema.Inputs[resource.PropertyKey(name)]
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
				actual, ok := awsVpcSchema.Outputs[resource.PropertyKey(name)]
				assert.True(t, ok, "output %s is missing from the schema", name)
				assert.Equal(t, expected.Description, actual.Description, "output %s description is incorrect", name)
				assert.Equal(t, expected.Secret, actual.Secret, "output %s secret is incorrect", name)
				assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "output %s type is incorrect", name)
			}
		})
	}
}

func TestParsingModuleSchemaOverrides(t *testing.T) {
	packageName := "vpc"
	overrides := parseModuleSchemaOverrides(packageName)
	assert.NotNil(t, overrides, "overrides is nil")

	var testSchemaOverride *ModuleSchemaOverride
	for _, override := range overrides {
		if override.Source == "example-module-source-for-testing" {
			testSchemaOverride = override
			break
		}
	}
	assert.NotNil(t, testSchemaOverride, "test schema override is nil")
	assert.NotNil(t, testSchemaOverride.MinimumVersion, "partial schema is nil")
	assert.Equal(t, *testSchemaOverride.MinimumVersion, "0.1.0", "minimum version is incorrect")
	assert.Equal(t, testSchemaOverride.MaximumVersion, "6.0.0", "maximum version is incorrect")
	assert.NotNil(t, testSchemaOverride.PartialSchema, "partial schema is nil")
	assert.Equal(t, testSchemaOverride.PartialSchema.Inputs, map[resource.PropertyKey]*schema.PropertySpec{
		"example_input": {
			Description: "An example input for the module.",
			TypeSpec:    stringType,
		},
		"example_ref": {
			TypeSpec: refType("#/types/vpc:index:MyType"),
		},
	})

	assert.Equal(t, testSchemaOverride.PartialSchema.Outputs, map[resource.PropertyKey]*schema.PropertySpec{
		"example_output": {
			TypeSpec:    boolType,
			Description: "An example output for the module.",
		},
	})

	assert.Equal(t, testSchemaOverride.PartialSchema.SupportingTypes, map[string]*schema.ComplexTypeSpec{
		"vpc:index:MyType": {
			ObjectTypeSpec: schema.ObjectTypeSpec{
				Type:        "object",
				Description: "An example supporting type for the module.",
				Properties: map[string]schema.PropertySpec{
					"example_property": {
						Description: "An example property for the supporting type.",
						TypeSpec:    stringType,
					},
				},
			},
		},
	})

	assert.Equal(t, testSchemaOverride.PartialSchema.RequiredInputs, []resource.PropertyKey{
		"example_input",
	})

	assert.Equal(t, testSchemaOverride.PartialSchema.NonNilOutputs, []resource.PropertyKey{
		"example_output",
	})
}

func TestApplyModuleOverrides(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("vpc")
	version := TFModuleVersion("5.21.0")
	source := TFModuleSource("terraform-aws-modules/vpc/aws")
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			awsVpcSchema, err := InferModuleSchema(ctx, tf, packageName, source, version)
			assert.NoError(t, err, "failed to infer module schema for aws vpc module")
			assert.NotNil(t, awsVpcSchema, "inferred module schema for aws vpc module is nil")
			// We cannot infer which outputs are required or not so everything is optional, initially.
			assert.Empty(t, awsVpcSchema.NonNilOutputs, "required outputs is empty")

			t.Run("required inputs/outputs are updated", func(t *testing.T) {
				moduleOverrides := []*ModuleSchemaOverride{
					{
						Source:         string(source),
						MaximumVersion: "6.0.0",
						PartialSchema: &InferredModuleSchema{
							NonNilOutputs:  []resource.PropertyKey{"vpc_id"},
							RequiredInputs: []resource.PropertyKey{"cidr"},
						},
					},
				}

				partialAwsVpcSchemaOverride, ok := hasBuiltinModuleSchemaOverrides(source, version, moduleOverrides)
				assert.True(t, ok, "module schema overrides should be found")
				overridenSchema := combineInferredModuleSchema(awsVpcSchema, partialAwsVpcSchemaOverride)
				assert.NotNil(t, overridenSchema, "overridden module schema is nil")
				assert.Contains(t, overridenSchema.NonNilOutputs, resource.PropertyKey("vpc_id"), "vpc_id should be required")
				assert.Contains(t, overridenSchema.RequiredInputs, resource.PropertyKey("cidr"), "cidr should be required")
			})

			t.Run("specific fields can be updated", func(t *testing.T) {
				moduleOverrides := []*ModuleSchemaOverride{
					{
						Source:         string(source),
						MaximumVersion: "6.0.0",
						PartialSchema: &InferredModuleSchema{
							Outputs: map[resource.PropertyKey]*schema.PropertySpec{
								"vpc_id": {
									Description: "The new ID field of the VPC",
									Secret:      true,
								},
							},
						},
					},
				}

				partialAwsVpcSchemaOverride, ok := hasBuiltinModuleSchemaOverrides(source, version, moduleOverrides)
				assert.True(t, ok, "module schema overrides should be found")
				overridenSchema := combineInferredModuleSchema(awsVpcSchema, partialAwsVpcSchemaOverride)
				assert.NotNil(t, overridenSchema, "overridden module schema is nil")
				assert.Contains(t, overridenSchema.NonNilOutputs, resource.PropertyKey("vpc_id"), "vpc_id should be non-nil")

				vpcID := overridenSchema.Outputs["vpc_id"]
				assert.Equal(t, "The new ID field of the VPC", vpcID.Description, "vpc_id description should be updated")
				assert.True(t, vpcID.Secret, "vpc_id should be secret")
				assert.Equal(t, "string", vpcID.TypeSpec.Type, "vpc_id type should not be changed")
				assert.Contains(t, overridenSchema.NonNilOutputs, resource.PropertyKey("vpc_id"), "vpc_id should be non-nil")
			})
		})
	}
}

func TestExtractModuleContentWorksFromLocalPath(t *testing.T) {
	ctx := context.Background()
	src := filepath.Join("..", "..", "tests", "testdata", "modules", "randmod")
	p, err := filepath.Abs(src)
	require.NoError(t, err)
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			logger := newTestLogger(t)
			tf := newTestRuntime(t, executor)
			assert.NoError(t, err, "failed to pick module runtime")
			mod, err := extractModuleContent(ctx, tf, TFModuleSource(p), "", logger)
			require.NoError(t, err)
			require.NotNil(t, mod, "module contents should not be nil")
		})
	}
}

func TestInferringInputsFromLocalPath(t *testing.T) {
	ctx := context.Background()
	src := filepath.Join("..", "..", "tests", "testdata", "modules", "schema-inference-example")
	p, err := filepath.Abs(src)
	require.NoError(t, err)
	for _, executor := range []string{"terraforn", "opentofu"} {
		logger := newTestLogger(t)
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			assert.NoError(t, err, "failed to pick module runtime")
			inferredSchema, err := inferModuleSchema(ctx, tf,
				packageName("schema-inference-example"),
				TFModuleSource(p),
				TFModuleVersion(""),
				logger)
			require.NoError(t, err)
			require.NotNil(t, inferredSchema, "module schema should not be nil")

			expectedInputs := map[resource.PropertyKey]*schema.PropertySpec{
				"required_string": {
					Description: "required string",
					TypeSpec:    stringType,
				},
				"optional_string_with_default": {
					Description: "optional string with default",
					TypeSpec:    stringType,
				},
				"optional_string_without_default": {
					Description: "optional string without default",
					TypeSpec:    stringType,
				},
				"required_string_using_nullable_false": {
					TypeSpec: stringType,
				},
				"optional_string_using_nullable_true": {
					TypeSpec: stringType,
				},
				"required_boolean": {
					TypeSpec: boolType,
				},
				"optional_boolean_with_default": {
					TypeSpec: boolType,
				},
				"required_number": {
					TypeSpec: numberType,
				},
				"optional_number_with_default": {
					TypeSpec: numberType,
				},
				"required_list_of_strings": {
					TypeSpec: arrayType(stringType),
				},
				"optional_list_of_strings_with_default": {
					TypeSpec:    arrayType(stringType),
					Description: "optional list of strings with default",
				},
				"optional_list_of_strings_without_default": {
					TypeSpec:    arrayType(stringType),
					Description: "optional list of strings without default",
				},
				"required_map_of_strings": {
					TypeSpec: mapType(stringType),
				},
				"optional_map_of_strings_with_default": {
					TypeSpec:    mapType(stringType),
					Description: "optional map of strings with default",
				},
			}

			for name, expected := range expectedInputs {
				actual, ok := inferredSchema.Inputs[name]
				assert.True(t, ok, "input %s is missing from the schema", name)
				assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
				assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
			}

			expectedRequiredInputs := []resource.PropertyKey{
				"required_string",
				"required_string_using_nullable_false",
				"required_number",
				"required_boolean",
				"required_list_of_strings",
				"required_map_of_strings",
			}

			actualRequiredInputs := inferredSchema.RequiredInputs

			slices.Sort(expectedRequiredInputs)
			slices.Sort(actualRequiredInputs)
			assert.Equal(t, expectedRequiredInputs, actualRequiredInputs)
		})
	}
}

func TestInferringSchemaWithDashedFielsFromLocalPath(t *testing.T) {
	ctx := context.Background()
	src := filepath.Join("..", "..", "tests", "testdata", "modules", "dashed-module-fields")
	p, err := filepath.Abs(src)
	require.NoError(t, err)
	for _, executor := range []string{"terraforn", "opentofu"} {
		logger := newTestLogger(t)
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			assert.NoError(t, err, "failed to pick module runtime")
			inferredSchema, err := inferModuleSchema(ctx, tf,
				packageName("dashed"),
				TFModuleSource(p),
				TFModuleVersion(""),
				logger)
			require.NoError(t, err)
			require.NotNil(t, inferredSchema, "module schema should not be nil")

			expectedInputs := map[resource.PropertyKey]*schema.PropertySpec{
				"dashed_input": {
					TypeSpec: stringType,
				},
			}

			// Pulumi -> Terraform field name conversion
			expectedInputFieldMappings := map[resource.PropertyKey]resource.PropertyKey{
				"dashed_input": "dashed-input",
			}

			assert.Equal(t, expectedInputs, inferredSchema.Inputs, "inferred inputs do not match")
			assert.Equal(t, expectedInputFieldMappings, inferredSchema.SchemaFieldMappings.InputFieldMappings)

			expectedOutputs := map[resource.PropertyKey]*schema.PropertySpec{
				"dashed_output": {
					TypeSpec: stringType,
				},
			}

			// Pulumi -> Terraform field name conversion
			expectedOutputFieldMappings := map[resource.PropertyKey]resource.PropertyKey{
				"dashed_output": "dashed-output",
			}

			assert.Equal(t, expectedOutputs, inferredSchema.Outputs, "inferred outputs do not match")
			assert.Equal(t, expectedOutputFieldMappings, inferredSchema.SchemaFieldMappings.OutputFieldMappings)

			expectedProviderConfig := schema.ConfigSpec{
				Variables: map[string]schema.PropertySpec{
					"google_beta": {
						Description: "provider configuration for google_beta",
						TypeSpec:    mapType(anyType),
					},
				},
			}

			expectedProviderFieldMappings := map[string]string{
				"google_beta": "google-beta",
			}

			assert.Equal(t, expectedProviderConfig, inferredSchema.ProvidersConfig, "inferred provider config does not match")
			assert.Equal(t, expectedProviderFieldMappings, inferredSchema.SchemaFieldMappings.ProviderFieldMappings,
				"inferred provider field mappings do not match")
		})
	}
}

func TestInferModuleSchemaFromGitHubSource(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("demoWebsite")
	version := TFModuleVersion("") // GitHub-sourced modules don't take a version

	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {

			tf := newTestRuntime(t, executor)
			demoSchema, err := InferModuleSchema(ctx,
				tf,
				packageName,
				"github.com/yemisprojects/s3_website_module_demo",
				version)

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
				actual, ok := demoSchema.Inputs[resource.PropertyKey(name)]
				assert.True(t, ok, "input %s is missing from the schema", name)
				assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
				assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
				assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
			}
		})
	}
}

func TestInferModuleSchemaFromGitHubSourceWithSubModule(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("consulCluster")
	version := TFModuleVersion("") // GitHub-sourced modules don't take a version
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			consulClusterSchema, err := InferModuleSchema(ctx,
				tf,
				packageName,
				"github.com/hashicorp/terraform-aws-consul//modules/consul-cluster",
				version)

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
				actual, ok := consulClusterSchema.Inputs[resource.PropertyKey(name)]
				assert.True(t, ok, "input %s is missing from the schema", name)
				assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
				assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
				assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
			}
		})
	}
}

func TestInferModuleSchemaFromGitHubSourceWithSubModuleAndVersion(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("consulCluster")
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			source := TFModuleSource("github.com/hashicorp/terraform-aws-consul//modules/consul-cluster?ref=v0.11.0")
			referencedVersion, ok := source.ReferencedVersionInURL()
			assert.True(t, ok, "referenced version should be found in the source URL")
			assert.Equal(t, "0.11.0", referencedVersion, "referenced version should be 0.11.0")
			consulClusterSchema, err := InferModuleSchema(ctx,
				tf,
				packageName,
				"github.com/hashicorp/terraform-aws-consul//modules/consul-cluster?ref=v0.11.0",
				TFModuleVersion(referencedVersion))

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
				actual, ok := consulClusterSchema.Inputs[resource.PropertyKey(name)]
				assert.True(t, ok, "input %s is missing from the schema", name)
				assert.Equal(t, expected.Description, actual.Description, "input %s description is incorrect", name)
				assert.Equal(t, expected.Secret, actual.Secret, "input %s secret is incorrect", name)
				assert.Equal(t, expected.TypeSpec, actual.TypeSpec, "input %s type is incorrect", name)
			}
		})
	}
}

func TestInferRequiredInputsWorks(t *testing.T) {
	ctx := context.Background()
	packageName := packageName("http")
	for _, executor := range []string{"terraform", "opentofu"} {
		t.Run("executor="+executor, func(t *testing.T) {
			source := TFModuleSource("terraform-aws-modules/security-group/aws//modules/http-80")
			version := TFModuleVersion("5.3.0")
			tf := newTestRuntime(t, executor)
			httpSchema, err := InferModuleSchema(ctx, tf, packageName, source, version)
			assert.NoError(t, err, "failed to infer module schema for aws vpc module")
			assert.NotNil(t, httpSchema, "inferred module schema for aws vpc module is nil")
			assert.Contains(t, httpSchema.RequiredInputs, resource.PropertyKey("vpc_id"))
		})
	}
}

func TestResolveModuleSources(t *testing.T) {
	executors := getExecutorsFromEnv()
	for _, executor := range executors {
		t.Run("executor="+executor, func(t *testing.T) {
			tf := newTestRuntime(t, executor)
			t.Run("local path-based module source", func(t *testing.T) {
				ctx := context.Background()
				src := filepath.Join("..", "..", "tests", "testdata", "modules", "randmod")
				p, err := filepath.Abs(src)
				require.NoError(t, err)
				d, err := resolveModuleSources(ctx, tf, TFModuleSource(p), "", tfsandbox.DiscardLogger)
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
				d, err := resolveModuleSources(ctx, tf, s, v, tfsandbox.DiscardLogger)
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
				workingDirectory, err := resolveModuleSources(ctx, tf, moduleSource, "", tfsandbox.DiscardLogger)
				require.NoError(t, err)

				bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
				require.NoError(t, err)

				t.Logf("variables.tf: %s", bytes)
				assert.Contains(t, string(bytes), "index_document")
			})

			t.Run("remote module source with version in source path", func(t *testing.T) {
				ctx := context.Background()
				moduleSource := TFModuleSource("github.com/yemisprojects/s3_website_module_demo?ref=v0.0.1")
				workingDirectory, err := resolveModuleSources(ctx, tf, moduleSource, "", tfsandbox.DiscardLogger)
				require.NoError(t, err)

				bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
				require.NoError(t, err)

				t.Logf("variables.tf: %s", bytes)
				assert.Contains(t, string(bytes), "index_document")
			})

			t.Run("remote module source with git path prefix", func(t *testing.T) {
				ctx := context.Background()
				moduleSource := TFModuleSource("git::github.com/yemisprojects/s3_website_module_demo?ref=v0.0.1")
				workingDirectory, err := resolveModuleSources(ctx, tf, moduleSource, "", tfsandbox.DiscardLogger)
				require.NoError(t, err)

				bytes, err := os.ReadFile(filepath.Join(workingDirectory, "variables.tf"))
				require.NoError(t, err)

				t.Logf("variables.tf: %s", bytes)
				assert.Contains(t, string(bytes), "index_document")
			})
		})
	}
}
