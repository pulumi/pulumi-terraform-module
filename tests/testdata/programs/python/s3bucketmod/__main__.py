"""A Python Pulumi program"""

import pulumi
import pulumi_bucket as bucket

config = pulumi.Config()
prefix = config.get('prefix') or pulumi.get_stack()

testbucket = bucket.Module("test-bucket",
    bucket=f"{prefix}-test-bucket",

)

pulumi.export('bucketARN', testbucket.s3_bucket_arn)
