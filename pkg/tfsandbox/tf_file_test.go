package tfsandbox

import (
	"bytes"
	"context"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"
)

func TestCreateTFFile(t *testing.T) {
	t.Parallel()

	t.Run("Simple", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		err = tofu.CreateTFFile("simple", "terraform-aws-modules/vpc/aws", "5.16.0", resource.PropertyMap{
			"cidr": resource.NewStringProperty("10.0.0.0/16"),
		})
		assert.NoError(t, err)

		var res bytes.Buffer
		err = tofu.tf.InitJSON(context.Background(), &res)
		assert.NoError(t, err)
		t.Logf("Output: %s", res.String())
	})

	t.Run("With lists", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		err = tofu.CreateTFFile("simple", "terraform-aws-modules/vpc/aws", "5.16.0", resource.PropertyMap{
			"cidr": resource.NewStringProperty("10.0.0.0/16"),
			"azs": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("us-west-2a"),
				resource.NewStringProperty("us-west-2b"),
			}),
		})
		assert.NoError(t, err)

		var res bytes.Buffer
		err = tofu.tf.InitJSON(context.Background(), &res)
		assert.NoError(t, err)
		t.Logf("Output: %s", res.String())

		assertValidateSuccess(t, tofu)
	})

	t.Run("With blocks", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		err = tofu.CreateTFFile("simple", "terraform-aws-modules/vpc/aws", "5.16.0", resource.PropertyMap{
			"cidr": resource.NewStringProperty("10.0.0.0/16"),
			"tags": resource.NewObjectProperty(resource.PropertyMap{
				"Name": resource.NewStringProperty("simple-vpc"),
			}),
		})
		assert.NoError(t, err)

		var res bytes.Buffer
		err = tofu.tf.InitJSON(context.Background(), &res)
		assert.NoError(t, err)
		t.Logf("Output: %s", res.String())

		assertValidateSuccess(t, tofu)
	})

	t.Run("Vpc complete example", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		t.Logf("WorkingDir: %s", tofu.WorkingDir())
		err = tofu.CreateTFFile("simple", "terraform-aws-modules/vpc/aws", "5.16.0", resource.PropertyMap{
			"cidr": resource.NewStringProperty("10.0.0.0/16"),
			"privateSubnets": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("10.0.101.0/24"),
				resource.NewStringProperty("10.0.102.0/24"),
				resource.NewStringProperty("10.0.103.0/24"),
			}),
			"createDatabaseSubnetGroup": resource.NewBoolProperty(false),
			"manageDefaultNetworkAcl":   resource.NewBoolProperty(false),
			"customerGateways": resource.NewObjectProperty(resource.PropertyMap{
				"IP1": resource.NewObjectProperty(resource.PropertyMap{
					"bgp_asn":     resource.NewNumberProperty(65112),
					"ip_address":  resource.NewStringProperty("1.2.3.4"),
					"device_name": resource.NewStringProperty("device1"),
				}),
				"IP2": resource.NewObjectProperty(resource.PropertyMap{
					"bgp_asn":    resource.NewNumberProperty(65112),
					"ip_address": resource.NewStringProperty("5.6.7.8"),
				}),
			}),
			"intraSubnetNames": resource.NewArrayProperty([]resource.PropertyValue{}),
			"tags": resource.NewObjectProperty(resource.PropertyMap{
				"Name": resource.NewStringProperty("simple-vpc"),
			}),
		})
		assert.NoError(t, err)

		var res bytes.Buffer
		err = tofu.tf.InitJSON(context.Background(), &res)
		assert.NoError(t, err)
		t.Logf("Output: %s", res.String())

		assertValidateSuccess(t, tofu)
	})

	t.Run("SecurityGroup complete example", func(t *testing.T) {
		tofu, err := NewTofu(context.Background())
		assert.NoError(t, err)
		t.Logf("WorkingDir: %s", tofu.WorkingDir())
		err = tofu.CreateTFFile("simple", "terraform-aws-modules/security-group/aws", "5.3.0", resource.PropertyMap{
			"name": resource.NewStringProperty("complete-sg"),
			"ingressCidrBlocks": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewStringProperty("10.0.101.0/24"),
				resource.NewStringProperty("10.0.102.0/24"),
				resource.NewStringProperty("10.0.103.0/24"),
			}),
			// TODO: [pulumi/pulumi-terraform-module-provider#28] support unknowns
			// "vpc_id": resource.MakeComputed(resource.NewStringProperty("")),
			"numberOfComputedIngressRules": resource.NewNumberProperty(1),
			"ingressWithCidrBlocks": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{
					"from_port":   resource.NewNumberProperty(0),
					"to_port":     resource.NewNumberProperty(65535),
					"protocol":    resource.NewStringProperty("tcp"),
					"cidr_blocks": resource.NewStringProperty("10.10.0.0/20"),
					"description": resource.NewStringProperty("allow all from"),
				}),
				resource.NewObjectProperty(resource.PropertyMap{
					"rule":        resource.NewStringProperty("postgresql-tcp"),
					"cidr_blocks": resource.NewStringProperty("0.0.0.0/0,2.2.2.2/32"),
				}),
			}),
			"computedIngressWithCidrBlocks": resource.NewArrayProperty([]resource.PropertyValue{
				resource.NewObjectProperty(resource.PropertyMap{
					"from_port": resource.NewNumberProperty(0),
					"to_port":   resource.NewNumberProperty(65535),
					"protocol":  resource.NewStringProperty("tcp"),
					// TODO: [pulumi/pulumi-terraform-module-provider#28] support unknowns
					// "cidr_blocks": resource.MakeComputed(resource.NewStringProperty("")),
					"description": resource.NewStringProperty("allow all from"),
				}),
			}),
		})
		assert.NoError(t, err)

		var res bytes.Buffer
		err = tofu.tf.InitJSON(context.Background(), &res)
		assert.NoError(t, err)
		t.Logf("Output: %s", res.String())

		assertValidateSuccess(t, tofu)
	})
}

// validate will fail if any of the module inputs don't match
// the schema of the module
func assertValidateSuccess(t *testing.T, tofu *Tofu) {
	val, err := tofu.tf.Validate(context.Background())
	assert.NoErrorf(t, err, "Tofu validation failed")
	assert.Equalf(t, true, val.Valid, "Tofu validation - expected valid=true, got valid=false")
	assert.Equalf(t, 0, val.ErrorCount, "Tofu validation - expected error count=0, got %d", val.ErrorCount)
	assert.Equalf(t, 0, val.WarningCount, "Tofu validation - expected warning count=0, got %d", val.WarningCount)

}
