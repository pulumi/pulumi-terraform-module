package main

import (
	"example.com/pulumi-bucket/sdk/go/v4/bucket"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		prefix := ctx.Stack()
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
