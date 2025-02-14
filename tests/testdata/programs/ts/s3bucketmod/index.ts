import * as pulumi from "@pulumi/pulumi";
import * as bucket from "@pulumi/bucket";


const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();

const testbucket = new bucket.Module("test-bucket", {
    bucket: `${prefix}-test-bucket`,
})

export const bucketARN =  testbucket.s3_bucket_arn