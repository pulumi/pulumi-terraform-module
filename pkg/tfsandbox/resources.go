package tfsandbox

import (
	"maps"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// ResourceAddress is the address of the resource given in the plan
// e.g. module.s3_bucket.aws_s3_bucket.this[0]
// OR aws_s3_bucket.this (depending on where in the plan it is)
type ResourceAddress string

// StateResource is a map of the resource address to the resource
type stateResources map[ResourceAddress]tfjson.StateResource

// newStateResources creates a new stateResources object from a tfjson.StateModule
func newStateResources(module *tfjson.StateModule) (stateResources, error) {
	resources := make(stateResources)
	if err := resources.extractResourcesFromStateModule(module); err != nil {
		return nil, err
	}
	return resources, nil
}

// extractResourcesFromStateModule extracts a list of resources from a tfjson.StateModule
// This can be either from the plan `PlannedValues` or the state `Values` (after apply is finished)
// The `PlannedValues` contains the final result of the plan, which includes all the resources
// that are going to be created, updated, deleted, replaced, or kept unchanged.
func (sr stateResources) extractResourcesFromStateModule(module *tfjson.StateModule) error {
	if module.ChildModules != nil {
		for _, childModule := range module.ChildModules {
			if err := sr.extractResourcesFromStateModule(childModule); err != nil {
				return err
			}
		}
	}

	for _, resource := range module.Resources {
		sr[ResourceAddress(resource.Address)] = *resource
	}

	return nil
}

// updateResourceValue updates the value of a resource property based on a filter (e.g. AfterSensitive, AfterUnknown)
// For example, AfterSensitive would contain a map of attributes keys with the value of true if the attribute is sensitive
//
// e.g.
//
//	 {
//		  "sensitive_values": {
//		    "access_key": true,
//	     "encryption_config": {
//	        "kms_key_id": true
//	     }
//	   }
//	 }
func updateResourceValue(old resource.PropertyValue, filter interface{}, replv func(v resource.PropertyValue) resource.PropertyValue) resource.PropertyValue {
	if old.IsArray() {
		arrValue := old.ArrayValue()
		if filterSlice, ok := filter.([]interface{}); ok {
			for i := range filterSlice {
				if i >= len(arrValue) {
					break
				}
				arrValue[i] = updateResourceValue(arrValue[i], filterSlice[i], replv)
			}
		}
		old = resource.NewArrayProperty(arrValue)
	}
	if old.IsObject() {
		objValue := old.ObjectValue()
		if filterMap, ok := filter.(map[string]interface{}); ok {
			for key := range maps.Keys(filterMap) {
				// if the key exists in the filter, but not in the property map then we need to add it.
				// This should only happen with AfterUnknown values because those are the only types of values
				// that can appear in the changes, but not in the StateResource.AttributeValues
				if _, ok := objValue[resource.PropertyKey(key)]; !ok {
					objValue[resource.PropertyKey(key)] = resource.NewNullProperty()
				}
			}
			for filterKey := range filterMap {
				if value, ok := objValue[resource.PropertyKey(filterKey)]; ok {
					objValue[resource.PropertyKey(filterKey)] = updateResourceValue(value, filterMap[filterKey], replv)
				}
			}
		}
		old = resource.NewObjectProperty(objValue)
	}

	if shouldFilter, ok := filter.(bool); ok && shouldFilter {
		return replv(old)
	}
	return old
}

// extractPropertyMapFromPlan extracts the property map from a tfjson.StateResource that is from a plan (PlannedValues)
// it takes care of updating the values of the resource based on the AfterSensitive and AfterUnknown values from the ResourceChange
func extractPropertyMapFromPlan(stateResource tfjson.StateResource, resourceChange *tfjson.ResourceChange) resource.PropertyMap {
	resourcePropertyMap := extractPropertyMap(stateResource)
	objectProperty := resource.NewObjectProperty(resourcePropertyMap)
	if resourceChange != nil && resourceChange.Change.AfterSensitive != nil {
		objectProperty = updateResourceValue(objectProperty, resourceChange.Change.AfterSensitive, func(v resource.PropertyValue) resource.PropertyValue {
			return resource.MakeSecret(v)
		})
	}
	if resourceChange != nil && resourceChange.Change.AfterUnknown != nil {
		objectProperty = updateResourceValue(objectProperty, resourceChange.Change.AfterUnknown, func(v resource.PropertyValue) resource.PropertyValue {
			if v.IsNull() {
				return resource.MakeComputed(resource.NewStringProperty(""))
			}
			return resource.MakeComputed(v)
		})
	}
	return objectProperty.ObjectValue()
}

// extractPropertyMapFromState extracts the property map from a tfjson.StateResource that is from a state (Values)
// it takes care of updating the values of the resource based on the SensitiveValues
func extractPropertyMapFromState(stateResource tfjson.StateResource) resource.PropertyMap {
	resourcePropertyMap := extractPropertyMap(stateResource)
	objectProperty := resource.NewObjectProperty(resourcePropertyMap)
	if stateResource.SensitiveValues != nil {
		objectProperty = updateResourceValue(objectProperty, stateResource.SensitiveValues, func(v resource.PropertyValue) resource.PropertyValue {
			return resource.MakeSecret(v)
		})
	}
	return objectProperty.ObjectValue()
}

// extractPropertyMap extracts the property map from a tfjson.StateResource
func extractPropertyMap(stateResource tfjson.StateResource) resource.PropertyMap {
	resourceConfig := resource.PropertyMap{}
	for attrKey, attrValue := range stateResource.AttributeValues {
		key := resource.PropertyKey(attrKey)
		resourceConfig[key] = resource.NewPropertyValue(attrValue)
	}
	return resourceConfig
}
