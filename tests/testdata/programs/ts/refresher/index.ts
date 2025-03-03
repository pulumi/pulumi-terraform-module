import * as pulumi from '@pulumi/pulumi';
import * as bucketmod from "@pulumi/bucketmod";

const cfg = new pulumi.Config();
const prefix = cfg.require("prefix");
const tagvalue = cfg.require("tagvalue");

new bucketmod.Module("mybucketmod", {
    prefix: prefix,
    tagvalue: tagvalue,
});
