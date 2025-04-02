import * as pulumi from '@pulumi/pulumi';
import * as mod from "@pulumi/mod";

const cfg = new pulumi.Config();

const keeper = cfg.require("keeper");

const m = new mod.Module("replacetestmod", {
    keeper: keeper,
});

export const randnum = m.randnum;
