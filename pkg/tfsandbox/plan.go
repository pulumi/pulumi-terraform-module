package tfsandbox

import (
	"context"
	"fmt"
	"path"
	"strings"

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

// The name of the Terraform module, e.g. if the module definition is
// module "s3_bucket" { ... }, then the module name is "s3_bucket"
type ModuleName string

// ResourceAddress is the address of the resource given in the plan
// e.g. module.s3_bucket.aws_s3_bucket.this[0]
// OR aws_s3_bucket.this (depending on where in the plan it is)
type ResourceAddress string

// PlanResources is a map of resource addresses to their converted properties
// TODO: A PropertyMap is not going to contain enough information for us to do everything
// we need to do (e.g. we need to know which properties are causing replacements). For now
// this gets us enough to do a RegisterResource call, but we will need to expand this later.
type PlanResources map[ResourceAddress]resource.PropertyMap

// ConvertedResources is a map of module names to their resources
type ConvertedResources map[ModuleName]PlanResources

type planConverter struct {
	convertedResources ConvertedResources
	finalResources     PlanResources
}

// convertModulesFromConfig takes the module Config from the plan and converts the resource
// properties to a pulumi PropertyMap.
//
// The difference between what is contained in the `Config` part of the plan vs what is contained
// in the `PlannedValues` or `ResourceChanges` is that the `Config` contains information about the
// actual Terraform configuration of the resource. This means it will tell us what properties have been
// populated with values (i.e. the resource inputs). The `PlannedValues` can contain information about
// both inputs and outputs.
//
// NOTE: Terraform plans show everything as inputs, even if they are optional computed values that the module didn't specify,
// whereas Pulumi plans show only the inputs that the module specified. We are attempting to do some filtering to
// make the Pulumi plan more Pulumi like, but if we want to instead just pass the plan through as is we could
// simplify this.
func (pc *planConverter) convertModulesFromConfig(moduleCalls map[string]*tfjson.ModuleCall) {
	for mName, call := range moduleCalls {
		if call.Module.ModuleCalls != nil {
			pc.convertModulesFromConfig(call.Module.ModuleCalls)
		}
		moduleResources := PlanResources{}
		for _, res := range call.Module.Resources {
			resourceProps := resource.PropertyMap{}
			addr := res.Address // e.g. aws_s3_bucket.this (note the lack of module prefix)
			for propertyKey, expression := range res.Expressions {
				key := resource.PropertyKey(propertyKey)
				// if the expression has references, then it might be a computed value
				// we will have more info later to determine for sure
				if expression.References != nil {
					resourceProps[key] = resource.MakeComputed(resource.NewStringProperty(""))
				} else {
					// otherwise just initialize it as a null property
					// we'll reassign it later if we find a value
					resourceProps[key] = resource.NewNullProperty()
				}
			}
			moduleResources[ResourceAddress(addr)] = resourceProps
		}
		pc.convertedResources[ModuleName(mName)] = moduleResources
	}
}

// convertModulesFromPlannedValue takes the module PlannedValues from the plan and generates the final
// list of resources with their converted properties.
//
// We only want to populate the value if it is associated with an input value to ensure
// the plan is more Pulumi like. (See note on convertModulesFromConfig)
func (pc *planConverter) convertModulesFromPlannedValue(module *tfjson.StateModule) error {
	if module.ChildModules != nil {
		for _, childModule := range module.ChildModules {
			if err := pc.convertModulesFromPlannedValue(childModule); err != nil {
				return err
			}
		}
	}
	moduleName := strings.Split(module.Address, "module.")
	if len(moduleName) != 2 {
		return fmt.Errorf("unexpected module address: %s", module.Address)
	}
	moduleResources, ok := pc.convertedResources[ModuleName(moduleName[1])]
	if !ok {
		return fmt.Errorf("module %s not found in the converted resources", moduleName[1])
	}
	for _, res := range module.Resources {
		resourceAddr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		resourceConfig, ok := moduleResources[ResourceAddress(resourceAddr)]
		if !ok {
			return fmt.Errorf("resource %s not found in the converted resources", resourceAddr)
		}

		addr := res.Address // e.g. module.s3_bucket.aws_s3_bucket.this[0]
		for attrKey, attrValue := range res.AttributeValues {
			if attrValue != nil {
				key := resource.PropertyKey(attrKey)
				// We only want to populate the value if it is associated with an input value
				// otherwise it's an optional computed value
				// On update plans for example, these will be populated from the state, but
				// we don't want to show those as inputs
				if _, ok := resourceConfig[key]; ok {
					resourceConfig[key] = resource.NewPropertyValue(attrValue)
				}
			}
		}
		pc.finalResources[ResourceAddress(addr)] = resourceConfig
	}
	return nil
}

// PulumiResourcesFromTFPlan process the Terraform plan and extracts information about the resources
func PulumiResourcesFromTFPlan(plan *tfjson.Plan) (PlanResources, error) {
	pc := &planConverter{
		convertedResources: ConvertedResources{},
		finalResources:     PlanResources{},
	}
	pc.convertModulesFromConfig(plan.Config.RootModule.ModuleCalls)

	for _, module := range plan.PlannedValues.RootModule.ChildModules {
		// The RootModule is the Terraform program itself. ChildModules contain the actual Terraform
		// Modules that are created in the Terraform program.
		// ChildModules[].Resources will contain all the individual resources created by the module
		if err := pc.convertModulesFromPlannedValue(module); err != nil {
			return nil, err
		}
	}
	return pc.finalResources, nil
}
