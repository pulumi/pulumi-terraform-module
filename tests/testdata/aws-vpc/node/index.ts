import * as pulumi from "@pulumi/pulumi";
import * as vpc from "@pulumi/vpc";

const defaultVpc = new vpc.Module("vpc", {cidr: "10.0.0.0/16"});
export const vpcId = defaultVpc.vpc_id;
