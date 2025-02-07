import * as pulumi from "@pulumi/pulumi";
import * as vpc from "@pulumi/vpc";

const vpc = new vpc.Module("vpc", {cidr: "10.0.0.0/16"});
export const vpcId = vpc.vpc_id;
