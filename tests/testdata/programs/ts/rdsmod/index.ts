import * as pulumi from "@pulumi/pulumi";
import * as rds from "@pulumi/rds";

const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();

const testrds = new rds.Module("test-rds", {
    identifier: `${prefix}test-rds-module`,
    engine: "mysql",
    instance_class: "db.t3.micro",
    allocated_storage: 20,
    db_name: "testrdsmoduledatabase",
    username: "pulumipus",
    password: "hawaii",
    skip_final_snapshot: true,
    deletion_protection: false,

    // DB parameter group
    family: "mysql8.0",

    // DB option group
    major_engine_version: "8.0",
})
