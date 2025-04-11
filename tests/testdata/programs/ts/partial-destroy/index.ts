import * as pulumi from "@pulumi/pulumi";
import * as localmod from '@pulumi/localmod';

const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();
const enabled = config.getBoolean('enabled') ?? true;

const mod = new localmod.Module('test-localmod', {
    name_prefix: prefix,
    enabled: enabled,
});

export const bucketName = mod.bucket_name;
export const bucketArn = mod.bucket_arn;
