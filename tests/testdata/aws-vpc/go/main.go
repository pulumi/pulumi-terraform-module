package main

import (
	"example.com/pulumi-terraform-aws-modules/sdk/go/v5/terraformawsmodules"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		vpc, err := terraformawsmodules.NewVpc(ctx, "vpc", &terraformawsmodules.VpcArgs{
			Cidr: pulumi.String("10.0.0.0/16"),
		})
		if err != nil {
			return err
		}
		ctx.Export("vpcId", vpc.Vpc_id)
		return nil
	})
}
