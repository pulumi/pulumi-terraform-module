import * as pulumi from "@pulumi/pulumi";
import * as rds from "@pulumi/rds";
import * as aws from "@pulumi/aws"

const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();


//Make a VPC
const vpc = new aws.ec2.Vpc("myVpc", {
    cidrBlock: "10.0.0.0/16",
    enableDnsHostnames: true,
    enableDnsSupport: true,
});

// Create a new security group within the VPC
const securityGroup = new aws.ec2.SecurityGroup("mySecurityGroup", {
    vpcId: vpc.id,
    description: "Allow MySQL access",
    ingress: [
        {
            protocol: "tcp",
            fromPort: 3306,
            toPort: 3306,
            cidrBlocks: ["0.0.0.0/0"],
        },
    ],
    egress: [
        {
            protocol: "-1",
            fromPort: 0,
            toPort: 0,
            cidrBlocks: ["0.0.0.0/0"],
        },
    ],
});

// Create a subnet for us-west-2a within the VPC
const subnet2a = new aws.ec2.Subnet("subnet2a", {
    vpcId: vpc.id,
    cidrBlock: "10.0.1.0/24",
    availabilityZone: "us-west-2a",
});

// Create a subnet for us-west-2b the VPC
const subnet2b = new aws.ec2.Subnet("subnet2b", {
    vpcId: vpc.id,
    cidrBlock: "10.0.2.0/24",
    availabilityZone: "us-west-2b",
})

const testrdsmodule = new rds.Module("test-rds", {
    identifier: `test-rds-module-${prefix}`,
    engine: "mysql",
    engine_version: "8.4",
    instance_class: "db.t3.micro",
    allocated_storage: 20,
    db_name: "testrdsmoduledatabase",
    username: "pulumipus",
    password: "hawaii",
    skip_final_snapshot: true,
    deletion_protection: false,


    // DB parameter group
    family: "mysql8.4",

    // DB subnet group
    create_db_subnet_group: true,
    subnet_ids: [subnet2a.id, subnet2b.id],

    // DB option group
    major_engine_version: "8.4",
    vpc_security_group_ids: [securityGroup.id]
})
