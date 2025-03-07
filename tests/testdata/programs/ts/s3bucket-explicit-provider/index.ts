import * as pulumi from "@pulumi/pulumi";
import * as bucket from "@pulumi/bucket";


const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();

const provider = new bucket.Provider("test-provider", {
    aws: {
        "region": "us-west-2"
    }
})

const testBucket = new bucket.Module("test-bucket", {
    bucket: `${prefix}-test-bucket`
}, { provider: provider });

export const bucketARN =  testBucket.s3_bucket_arn
