package modprovider

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/pulumi/pulumi-terraform-module/pkg/tfsandbox"
)

type (
	ResourceAddress     = tfsandbox.ResourceAddress
	TFResourceType      = tfsandbox.TFResourceType
	ChangeKind          = tfsandbox.ChangeKind
	ResourceStateOrPlan = tfsandbox.ResourceStateOrPlan
)

type Resource interface {
	Address() ResourceAddress

	// The resource type, example: "aws_instance" for aws_instance.foo.
	Type() TFResourceType

	// The resource name, example: "foo" for aws_instance.foo.
	Name() string
}

var _ Resource = (*tfsandbox.Resource)(nil)

type ResourcePlan interface {
	Resource
	ChangeKind() ChangeKind
	PlannedValues() resource.PropertyMap
}

var _ ResourcePlan = (*tfsandbox.ResourcePlan)(nil)

type ResourceState interface {
	Resource
	AttributeValues() resource.PropertyMap
}

type Resources interface {
	FindResourceStateOrPlan(ResourceAddress) (tfsandbox.ResourceStateOrPlan, bool)
	VisitResourcesStateOrPlans(func(ResourceStateOrPlan))
}

var _ ResourceState = (*tfsandbox.ResourceState)(nil)

type Plan interface {
	Resources

	Outputs() resource.PropertyMap
}

var _ Plan = (*tfsandbox.Plan)(nil)

type State interface {
	Resources // returns ResourceStateOrPlan=ResourceState
	Outputs() resource.PropertyMap
}

var _ State = (*tfsandbox.State)(nil)
