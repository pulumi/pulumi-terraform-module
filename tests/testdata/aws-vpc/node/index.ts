import * as pulumi from "@pulumi/pulumi";
import * as terraform_aws_modules from "@pulumi/terraform-aws-modules";

const vpc = new terraform_aws_modules.Vpc("vpc", {cidr: "10.0.0.0/16"});
export const vpcId = vpc.vpc_id;
