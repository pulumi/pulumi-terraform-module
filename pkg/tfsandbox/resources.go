package tfsandbox

import (
	"slices"

	tfjson "github.com/hashicorp/terraform-json"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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

// mapReplv maps the values of a resource property based on a filter
// The filter is an object that contains the keys of the attributes that might need to be updated
//
// There are cases where the filter contains a nested object, but the PropertyValue does not.
// In those cases we should update the PropertyValue to contain the nested object _only if_ the
// filter marks a nested value as true
//
// NOTE: This has array handling for completeness, but I don't think Terraform ever has detailed
// information on arrays. It seems to be the case that if any element in the array is sensitive or unknown
// then the entire array is marked as such. This makes sense because I don't think it is possible to guarantee the order
// of the elements in the array (i.e. the unknown value could return 1 item or 10).
func mapReplv(filter interface{}, old resource.PropertyValue, replv func(resource.PropertyValue) resource.PropertyValue) (resource.PropertyValue, bool) {
	contract.Assertf(!old.IsArchive() && !old.IsAsset() && !old.IsResourceReference() && !old.IsSecret(), "Archive, Asset, Secret, and Resource references are not expected here")
	switch f := filter.(type) {
	case bool:
		if f {
			return replv(old), true
		}
		return old, false
	case map[string]interface{}:
		objValue := resource.PropertyMap{}
		if old.IsObject() {
			objValue = old.ObjectValue()
		}
		var containsFilter bool
		for key, filterVal := range f {
			// if ok == false it means that there are no nested values in the PropertyValue that need to be updated
			if mapped, ok := mapReplv(filterVal, objValue[resource.PropertyKey(key)], replv); ok {
				containsFilter = true
				objValue[resource.PropertyKey(key)] = mapped
			}
		}
		return resource.NewObjectProperty(objValue), containsFilter
	case []interface{}:
		arrValue := make([]resource.PropertyValue, len(f))
		if old.IsArray() {
			oldArray := old.ArrayValue()
			if len(oldArray) < len(arrValue) {
				arrValue = slices.Replace(arrValue, 0, len(oldArray)-1, oldArray...)
			} else {
				arrValue = oldArray
			}
		}
		var containsFilter bool
		for i := range f {
			var value resource.PropertyValue
			if i >= len(arrValue) {
				value = resource.NewNullProperty()
			} else {
				value = arrValue[i]
			}
			if mapped, ok := mapReplv(f[i], value, replv); ok {
				containsFilter = true
				arrValue[i] = mapped
			}
		}
		arrValue = slices.DeleteFunc(arrValue, func(v resource.PropertyValue) bool {
			return v.IsNull()
		})
		return resource.NewArrayProperty(arrValue), containsFilter
	}
	return old, true
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
	if val, ok := mapReplv(filter, old, replv); ok {
		return val
	}

	return old
}

// extractPropertyMapFromPlan extracts the property map from a tfjson.StateResource that is from a plan (PlannedValues)
// it takes care of updating the values of the resource based on the AfterSensitive and AfterUnknown values from the ResourceChange
func extractPropertyMapFromPlan(stateResource tfjson.StateResource, resourceChange *tfjson.ResourceChange) resource.PropertyMap {
	resourcePropertyMap := extractPropertyMap(stateResource)
	objectProperty := resource.NewObjectProperty(resourcePropertyMap)
	if resourceChange != nil && resourceChange.Change.AfterUnknown != nil {
		objectProperty = updateResourceValue(objectProperty, resourceChange.Change.AfterUnknown, func(v resource.PropertyValue) resource.PropertyValue {
			return resource.MakeComputed(resource.NewStringProperty(""))
		})
	}

	if resourceChange != nil && resourceChange.Change.AfterSensitive != nil {
		objectProperty = updateResourceValue(objectProperty, resourceChange.Change.AfterSensitive, resource.MakeSecret)
	}
	return objectProperty.ObjectValue()
}

// extractPropertyMapFromState extracts the property map from a tfjson.StateResource that is from a state (Values)
// it takes care of updating the values of the resource based on the SensitiveValues
func extractPropertyMapFromState(stateResource tfjson.StateResource) resource.PropertyMap {
	resourcePropertyMap := extractPropertyMap(stateResource)
	objectProperty := resource.NewObjectProperty(resourcePropertyMap)
	if stateResource.SensitiveValues != nil {
		objectProperty = updateResourceValue(objectProperty, stateResource.SensitiveValues, resource.MakeSecret)
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
