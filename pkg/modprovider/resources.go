package modprovider

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"

	"github.com/pulumi/pulumi-terraform-module-provider/pkg/tfsandbox"
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
}

var _ ResourceState = (*tfsandbox.ResourceState)(nil)

type Plan interface {
	Resources
}

var _ Plan = (*tfsandbox.Plan)(nil)

type State interface {
	Resources // returns ResourceStateOrPlan=ResourceState
}

var _ State = (*tfsandbox.State)(nil)
