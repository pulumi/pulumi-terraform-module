import * as pulumi from '@pulumi/pulumi';
import * as bucketmod from "@pulumi/bucketmod";

const cfg = new pulumi.Config();
const prefix = cfg.require("prefix");
const tagvalue = cfg.require("tagvalue");

const m = new bucketmod.Module("mybucketmod", {
    prefix: prefix,
    tagvalue: tagvalue,
});

export const tags = m.tags;
