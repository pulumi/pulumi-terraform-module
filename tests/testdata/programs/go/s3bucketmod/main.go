package main

import (
	"github.com/pulumi/pulumi-terraform-module/sdks/bucket/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {

	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		prefix := cfg.Get("prefix")
		if prefix == "" {
			prefix = ctx.Stack()
		}
		testbucket, err := bucket.NewModule(ctx, "test-bucket", &bucket.ModuleArgs{
			Bucket: pulumi.String(prefix + "-test-bucket"),
		})
		if err != nil {
			return err
		}

		ctx.Export("bucketARN", testbucket.S3_bucket_arn)
		return nil
	})
}
