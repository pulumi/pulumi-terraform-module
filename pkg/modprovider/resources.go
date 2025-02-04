package modprovider

import (
	"github.com/pulumi/pulumi-terraform-module-provider/pkg/tfsandbox"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
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

	// The instance key for any resources that have been created using
	// "count" or "for_each". If neither of these apply the key will be
	// empty.
	//
	// This value can be either an integer (int) or a string.
	Index() interface{}
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

type Resources[T any] interface {
	VisitResources(func(T))
	FindResource(ResourceAddress) (T, bool)
}

var _ ResourceState = (*tfsandbox.ResourceState)(nil)

type Plan[T ResourcePlan] interface {
	Resources[T]
}

var _ Plan[*tfsandbox.ResourcePlan] = (*tfsandbox.Plan)(nil)

type State[T ResourceState] interface {
	Resources[T]
}

var _ State[*tfsandbox.ResourceState] = (*tfsandbox.State)(nil)
