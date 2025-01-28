import * as mod from "@pulumi/terraform-module-provider";

const vpc = new mod.VpcAws("my-vpc", {
    cidr: "10.0.0.0/16",
});

export const defaultVpcId = vpc.defaultVpcId;
