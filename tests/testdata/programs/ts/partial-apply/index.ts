import * as pulumi from "@pulumi/pulumi";
import * as localmod from '@pulumi/localmod';

const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();
const step = config.getNumber('step') ?? 1;

const mod = new localmod.Module('test-localmod', {
    name_prefix: prefix,
    should_fail: step === 1 ? true : false,
});

export const roleArn =  mod.role_arn;

