import * as pulumi from "@pulumi/pulumi";
import * as bucket from "@pulumi/bucket";


const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();

const testbucket = new bucket.Module("test-bucket", {
    bucket: `${prefix}-test-bucket`,
    // server_side_encryption_configuration: {
    //     rule: {
    //         apply_server_side_encryption_by_default: {
    //             sse_algorithm: pulumi.secret("AES256")
    //         },
    //     }
    // },
})

export const bucketARN =  testbucket.s3_bucket_arn
