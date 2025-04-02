import * as pulumi from '@pulumi/pulumi';
import * as replacetestmod from "@pulumi/replacetestmod";

const cfg = new pulumi.Config();

const keeper = cfg.require("keeper");

const m = new replacetestmod.Module("replacetestmod", {
    keeper: keeper,
});

export const randnum = m.randnum;
