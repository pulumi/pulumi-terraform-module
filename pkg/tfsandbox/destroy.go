package tfsandbox

import (
	"context"

	tfjson "github.com/hashicorp/terraform-json"
)

// Destroy runs the terraform destroy command
// This handles a couple of special cases around destroy.
// The returned State and Error can both be not nil. This would be the case
// if the destroy failed and some resources were destroyed and some were not.
//
// There are two special cases that callers should be aware of:
//  1. If the destroy was successful, we return a non-nil state with no resources
//  2. If the destroy failed, we either return the state with the resources that failed to destroy
//     or a nil state if something went really wrong and there is not state left
func (t *Tofu) Destroy(ctx context.Context, log Logger) (*State, error) {
	rawState, success, destroyErr := t.destroy(ctx, log)
	if success {
		s, err := newDestroyState(rawState)
		if err != nil {
			return nil, err
		}
		return s, nil
	}
	if rawState != nil {
		s, err := newDestroyState(rawState)
		if err != nil {
			return nil, err
		}
		return s, destroyErr
	}
	return nil, destroyErr
}

// Destroy runs the terraform destroy command
// This handles a couple of special cases around destroy.
//  1. If the destroy was successful, we return a nil state, success = true, nil error
//  2. If the destroy failed, then we have 2 possibilities
//     a. Show fails, either because we have a nil state or something else.
//     In this case, we return a nil state, success = false, and the error.
//     callers need to handle this case and decide what to do (probably just fail)
//     b. Show succeeds, in this case we return the state, success = false, and the error.
//     This would most likely be a partial failure where some resources were destroyed
//     and some were not. The resource that were not destroyed would still exist in the state
func (t *Tofu) destroy(ctx context.Context, log Logger) (state *tfjson.State, succeeded bool, err error) {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	err = t.tf.DestroyJSON(ctx, logWriter)
	if err == nil {
		return nil, true, nil
	}
	var showErr error
	state, showErr = t.Show(ctx)
	if showErr != nil {
		return nil, false, showErr
	}

	return state, false, err
}

// newDestroyState creates a new destroy state from the raw state
// This handles cases where the rawState is nil or the root values/module is nil
// which would happen if the destroy was successful
func newDestroyState(rawState *tfjson.State) (*State, error) {
	newT := func(resource tfjson.StateResource) *ResourceState {
		return &ResourceState{
			Resource: Resource{
				sr:    resource,
				props: extractPropertyMapFromState(resource),
			},
		}
	}
	if rawState == nil || rawState.Values == nil || rawState.Values.RootModule == nil {
		return &State{
			Resources: Resources[*ResourceState]{
				resources: stateResources{},
				newT:      newT,
			},
			rawState: rawState,
		}, nil
	}
	resources, err := newStateResources(rawState.Values.RootModule)
	if err != nil {
		return nil, err
	}
	return &State{
		Resources: Resources[*ResourceState]{
			resources: resources,
			newT:      newT,
		},
		rawState: rawState,
	}, nil
}
