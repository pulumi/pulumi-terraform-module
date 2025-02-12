import * as pulumi from "@pulumi/pulumi";
import * as bucket from "@pulumi/bucket";



const testbucket = new bucket.Module("test-bucket", {
    bucket: "guinstestbucket",
})

export const bucketARN =  testbucket.s3_bucket_arn