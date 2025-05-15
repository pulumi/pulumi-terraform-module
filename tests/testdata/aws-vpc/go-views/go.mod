module aws-vpc

go 1.23

require (
	github.com/pulumi/pulumi/sdk/v3 v3.30.0
	github.com/pulumi/pulumi-terraform-module/sdks/go/vpc/v5 v5.18.1
)

replace github.com/pulumi/pulumi-terraform-module/sdks/go/vpc/v5 => ./sdks/vpc
