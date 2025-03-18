using Pulumi;
using Pulumi.Bucket;
using System.Threading.Tasks;

class Program
{
    static Task Main()
    {
        return Deployment.RunAsync(() =>
        {
            // Create your stack here inside RunAsync

            var myBucketModule = new Module("myBucketModule", new ModuleArgs
            {
                Bucket = "guins-test-bucket",  // Example argument
                // Add other arguments here
            });

            return Task.CompletedTask;
        });
    }
}






//class Program
//{
//    static void Main()
//    {
////        var config = new Config();
////        var prefix = config.Get("prefix") ?? Deployment.Instance.StackName;
//
//        var testBucket = new Pulumi.Bucket.Module("test-bucket", new Pulumi.Bucket.ModuleArgs
//        {
//            Bucket = "guin-test-bucket",
//        });
//
////        // Export the bucket ARN
////        var bucketArn = testBucket.Arn.Apply(arn => arn);
////        Deployment.Instance.Export("bucketARN", bucketArn);
//    }
//}
