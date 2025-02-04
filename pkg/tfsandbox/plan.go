package tfsandbox

import (
	"context"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// Plan runs a Terraform plan and returns the plan output json
func (t *Tofu) Plan(ctx context.Context) (*tfjson.Plan, error) {
	planFile := path.Join(t.WorkingDir(), "plan.out")
	_ /*hasChanges*/, err := t.tf.Plan(ctx, tfexec.Out(planFile))
	if err != nil {
		return nil, fmt.Errorf("error running plan: %w", err)
	}

	plan, err := t.tf.ShowPlanFile(ctx, planFile)
	if err != nil {
		return nil, fmt.Errorf("error running show plan: %w", err)
	}

	return plan, nil
}

// ResourceAddress is the address of the resource given in the plan
// e.g. module.s3_bucket.aws_s3_bucket.this[0]
// OR aws_s3_bucket.this (depending on where in the plan it is)
type ResourceAddress string

// PlanResources is a map of resource addresses to their converted properties
// TODO: A PropertyMap is not going to contain enough information for us to do everything
// we need to do (e.g. we need to know which properties are causing replacements). For now
// this gets us enough to do a RegisterResource call, but we will need to expand this later.
type PlanResources map[ResourceAddress]resource.PropertyMap

type planConverter struct {
	finalResources PlanResources
}

// extractResourcesFromPlannedValues extracts a list of resources from the planned values.
// The `PlannedValues` contains the final result of the plan, which includes all the resources
// that are going to be created, updated, deleted, replaced, or kept unchanged.
// The `AttributeValues` of each resource contains the final values of the resource properties,
// but does not contain all the information we will eventually need.
//
// Couple of issues with `AttributeValues`:
// - It contains both the properties that are input to the resource as well as properties that are optional/computed (i.e. output values)
// - It does not contain information on the secret or unknown status of the propertie
// - It does not contain information on the diff status of the properties (e.g. whether the property is causing a replacement)
func (pc *planConverter) extractResourcesFromPlannedValues(module *tfjson.StateModule) error {
	if module.ChildModules != nil {
		for _, childModule := range module.ChildModules {
			if err := pc.extractResourcesFromPlannedValues(childModule); err != nil {
				return err
			}
		}
	}

	for _, res := range module.Resources {
		resourceConfig := resource.PropertyMap{}
		for attrKey, attrValue := range res.AttributeValues {
			key := resource.PropertyKey(attrKey)
			if attrValue != nil {
				resourceConfig[key] = resource.NewPropertyValue(attrValue)
				continue
			}
			// if it is nil, it means the property is unknown or unset, we don't know which based on this info
			resourceConfig[key] = resource.MakeComputed(resource.NewStringProperty(""))
		}
		pc.finalResources[ResourceAddress(res.Address)] = resourceConfig
	}
	return nil
}

// PulumiResourcesFromTFPlan process the Terraform plan and extracts information about the resources
func PulumiResourcesFromTFPlan(plan *tfjson.Plan) (PlanResources, error) {
	pc := &planConverter{
		finalResources: PlanResources{},
	}

	for _, module := range plan.PlannedValues.RootModule.ChildModules {
		// The RootModule is the Terraform program itself. ChildModules contain the actual Terraform
		// Modules that are created in the Terraform program.
		// ChildModules[].Resources will contain all the individual resources created by the module
		if err := pc.extractResourcesFromPlannedValues(module); err != nil {
			return nil, err
		}
	}
	return pc.finalResources, nil
}
