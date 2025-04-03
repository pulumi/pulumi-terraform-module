import * as pulumi from '@pulumi/pulumi';
import * as rmod from "@pulumi/rmod";

const config = new pulumi.Config();

const m = new rmod.Module("rmod", {
    pwd: config.require("pwd")
});

export const content = m.content;
