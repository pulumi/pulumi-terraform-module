package modprovider

import (
	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

type (
	ResourceAddress = tfsandbox.ResourceAddress
	TFResourceType  = tfsandbox.TFResourceType
	ChangeKind      = tfsandbox.ChangeKind
	ResourcePlan    = *tfsandbox.ResourcePlan
	ResourceState   = *tfsandbox.ResourceState
	Plan            = *tfsandbox.Plan
	State           = *tfsandbox.State
)
