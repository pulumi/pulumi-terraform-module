package main

import (
	"github.com/pulumi/pulumi-terraform-module/sdks/vpc"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		defaultVpc, err := vpc.NewModule(ctx, "defaultVpc", &vpc.ModuleArgs{
			Cidr: pulumi.String("10.0.0.0/16"),
		})
		if err != nil {
			return err
		}
		ctx.Export("vpcId", defaultVpc.Vpc_id)
		return nil
	})
}
