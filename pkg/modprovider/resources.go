package modprovider

import (
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

type ResourceAddress string

// Represents the TF resource type, example: "aws_instance" for aws_instance.foo.
type TFResourceType string

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

type Resources[T any] interface {
	VisitResources(func(T))
	FindResource(ResourceAddress) (T, bool)
}

func MustFindResource[T any](collection Resources[T], addr ResourceAddress) T {
	r, ok := collection.FindResource(addr)
	contract.Assertf(ok, "Failed to find a resource at %q", addr)
	return r
}

type ChangeKind int

const (
	NoOp ChangeKind = iota + 1
	Update
	Replace
	ReplaceDestroyBeforeCreate
	Create
	Read
	Delete
	Forget
)

type ResourcePlan interface {
	Resource
	ChangeKind() ChangeKind
	PlannedValues() resource.PropertyMap
}

type ResourceState interface {
	Resource
	AttributeValues() resource.PropertyMap
}

type ResourceStateOrPlan struct {
	State ResourceState
	Plan  ResourcePlan
}

func (sop *ResourceStateOrPlan) Resource() Resource {
	if sop.State != nil {
		return sop.State
	}
	return sop.Plan
}

func (sop *ResourceStateOrPlan) Values() resource.PropertyMap {
	if sop.State != nil {
		return sop.State.AttributeValues()
	}
	return sop.Plan.PlannedValues()
}

type Plan interface {
	Resources[ResourcePlan]
}

type State interface {
	Resources[ResourceState]
}
