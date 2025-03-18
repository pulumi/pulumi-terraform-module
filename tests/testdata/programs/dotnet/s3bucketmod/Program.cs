using Pulumi;
using Pulumi.Bucket;

class Program
{
    static void Main()
    {
        var config = new Config();
        var prefix = config.Get("prefix") ?? Deployment.Instance.StackName;

        var testBucket = new Pulumi.Bucket("test-bucket", new Pulumi.Bucket.BucketArgs
        {
            BucketName = $"{prefix}-test-bucket",
        });

//        // Export the bucket ARN
//        var bucketArn = testBucket.Arn.Apply(arn => arn);
//        Deployment.Instance.Export("bucketARN", bucketArn);
    }
}
