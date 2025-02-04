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

package tfsandbox

import (
	"context"
	"fmt"
	"path"

	"github.com/hashicorp/terraform-exec/tfexec"
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// Plan runs terraform plan and returns the plan representation.
func (t *Tofu) Plan(ctx context.Context) (*Plan, error) {
	plan, err := t.plan(ctx)
	if err != nil {
		return nil, err
	}
	return newPlan(plan), nil
}

func (t *Tofu) plan(ctx context.Context) (*tfjson.Plan, error) {
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
type planResources map[ResourceAddress]resource.PropertyMap

type planConverter struct {
	finalResources planResources
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
		resourceConfig := extractPropertyMap(res)
		pc.finalResources[ResourceAddress(res.Address)] = resourceConfig
	}
	return nil
}

// This is not suitable for previews as it will not have unknowns.
func extractPropertyMap(stateResource *tfjson.StateResource) resource.PropertyMap {
	resourceConfig := resource.PropertyMap{}
	// TODO respect stateResource.SensitiveValues
	for attrKey, attrValue := range stateResource.AttributeValues {
		key := resource.PropertyKey(attrKey)
		if attrValue != nil {
			resourceConfig[key] = resource.NewPropertyValue(attrValue)
			continue
		}
		// if it is nil, it means the property is unknown or unset, we don't know which based on this info
		resourceConfig[key] = resource.MakeComputed(resource.NewStringProperty(""))
	}
	return resourceConfig
}

// pulumiResourcesFromTFPlan process the Terraform plan and extracts information about the resources
func pulumiResourcesFromTFPlan(plan *tfjson.Plan) (planResources, error) {

	pc := &planConverter{
		finalResources: planResources{},
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
