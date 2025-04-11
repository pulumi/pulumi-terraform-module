package tfsandbox

import (
	"context"
	"fmt"

	tfjson "github.com/hashicorp/terraform-json"
)

// Destroy runs the terraform destroy command
// This handles a couple of special cases around destroy.
// The returned State and Error can both be not nil. This would be the case
// if the destroy failed and some resources were destroyed and some were not.
//
// There are two special cases that callers should be aware of:
//  1. If the destroy was successful, we return a non-nil state with no resources
//  2. If the destroy failed, we return the state with the resources that failed to destroy
func (t *Tofu) Destroy(ctx context.Context, log Logger) (*State, error) {
	var rawState *tfjson.State
	rawState, destroyErr := t.destroy(ctx, log)
	state, err := newDestroyState(rawState)
	if err != nil {
		log.Log(ctx, Debug, fmt.Sprintf("error creating destroy state: %v", err))
		return state, err
	}
	return state, destroyErr
}

// Destroy runs the terraform destroy command
// This handles a couple of special cases around destroy.
//  1. If the destroy was successful, we return the state, nil error
//  2. If the destroy failed, then we have 2 possibilities
//     a. Show fails, either because we have a nil state or something else.
//     In this case, we return a nil state, and the error.
//     callers need to handle this case and decide what to do (probably just fail)
//     b. Show succeeds, in this case we return the state, and the error.
//     This would most likely be a partial failure where some resources were destroyed
//     and some were not. The resource that were not destroyed would still exist in the state
func (t *Tofu) destroy(ctx context.Context, log Logger) (state *tfjson.State, err error) {
	logWriter := newJSONLogPipe(ctx, log)
	defer logWriter.Close()

	err = t.tf.DestroyJSON(ctx, logWriter)
	if err != nil {
		log.Log(ctx, Debug, fmt.Sprintf("error running tofu destroy: %v", err))
	}
	var showErr error
	state, showErr = t.Show(ctx)
	if showErr != nil {
		log.Log(ctx, Debug, fmt.Sprintf("error running tofu show: %v", showErr))
		return nil, showErr
	}

	return state, err
}

// newDestroyState creates a new destroy state from the raw state
// This handles cases where the rawState is nil or the root values/module is nil
// which would happen if the destroy was successful
//
// Note that the returned State will never be nil
func newDestroyState(rawState *tfjson.State) (*State, error) {
	if rawState == nil {
		return emptyState(newT), nil
	}
	var resources stateResources
	var err error
	if rawState.Values == nil || rawState.Values.RootModule == nil {
		resources = stateResources{}
	} else {
		resources, err = newStateResources(rawState.Values.RootModule)
		if err != nil {
			return emptyState(newT), err
		}
	}
	return &State{
		Resources: Resources[*ResourceState]{
			resources: resources,
			newT:      newT,
		},
		rawState: rawState,
	}, nil
}
