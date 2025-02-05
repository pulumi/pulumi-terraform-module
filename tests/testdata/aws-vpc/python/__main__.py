import pulumi
import pulumi_terraform_aws_modules as terraform_aws_modules

vpc = terraform_aws_modules.Vpc("vpc", cidr="10.0.0.0/16")
pulumi.export("vpcId", vpc.vpc_id)
