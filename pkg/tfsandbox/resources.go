package tfsandbox

import (
	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// ResourceAddress is the address of the resource given in the plan
// e.g. module.s3_bucket.aws_s3_bucket.this[0]
// OR aws_s3_bucket.this (depending on where in the plan it is)
type ResourceAddress string

// StateResource is a map of the resource address to the resource
type stateResources map[ResourceAddress]Resource

func newStateResources(module *tfjson.StateModule, resourceChanges map[ResourceAddress]*tfjson.ResourceChange) (stateResources, error) {
	resources := make(stateResources)
	if err := resources.extractResourcesFromStateModule(module, resourceChanges); err != nil {
		return nil, err
	}
	return resources, nil
}

// extractResourcesFromStateModule extracts a list of resources from a tfjson.StateModule
// This can be either from the plan `PlannedValues` or the state `Values` (after apply is finished)
// The `PlannedValues` contains the final result of the plan, which includes all the resources
// that are going to be created, updated, deleted, replaced, or kept unchanged.
//
// The `AttributeValues` of each resource contains the final values of the resource properties
// If we are in a plan, then `AttributeValues` might not contain the information we need on unknown
// values so we need to augment with the `ResourceChange` data.
func (sr stateResources) extractResourcesFromStateModule(module *tfjson.StateModule, resourceChanges map[ResourceAddress]*tfjson.ResourceChange) error {
	if module.ChildModules != nil {
		for _, childModule := range module.ChildModules {
			if err := sr.extractResourcesFromStateModule(childModule, resourceChanges); err != nil {
				return err
			}
		}
	}

	for _, res := range module.Resources {
		resourceConfig := extractPropertyMap(res, resourceChanges[ResourceAddress(res.Address)])
		sr[ResourceAddress(res.Address)] = Resource{
			sr:    *res,
			props: resourceConfig,
		}
	}
	return nil
}

func extractPropertyMap(stateResource *tfjson.StateResource, resourceChange *tfjson.ResourceChange) resource.PropertyMap {
	resourceConfig := resource.PropertyMap{}
	// TODO: [pulumi/pulumi-terraform-module-provider#45] respect stateResource.SensitiveValues
	for attrKey, attrValue := range stateResource.AttributeValues {
		key := resource.PropertyKey(attrKey)
		resourceConfig[key] = resource.NewPropertyValue(attrValue)
	}

	// If we have a resource change for this resource and `AfterUnknown` is populated
	// then we need to add these unknowns to the properties as computed values
	if resourceChange != nil && resourceChange.Change.AfterUnknown != nil {
		if after, ok := resourceChange.Change.AfterUnknown.(map[string]interface{}); ok {
			for attrKey, attrValue := range after {
				// The docs for `AfterUnknown` say that is is a _deep_ object of booleans, but from what I've
				// seen it has always been a map[string]bool. If there is an object where a nested attribute is unknown
				// it will mark the entire object as unknown (TestProcessPlan has an example).
				// TODO: [pulumi/pulumi-terraform-module-provider#88] handle nested unknowns
				if isUnknown, ok := attrValue.(bool); ok && isUnknown {
					resourceConfig[resource.PropertyKey(attrKey)] = resource.MakeComputed(resource.NewStringProperty(""))
				}
			}
		}
	}
	return resourceConfig
}
