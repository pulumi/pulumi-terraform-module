import * as pulumi from "@pulumi/pulumi";
import * as terraformAwsModules from "@pulumi/terraform-aws-modules";



const testbucket = new terraformAwsModules.S3_Bucket("test-bucket", {
    function_name: "guinstestbucket",

})

export const bucketARN =  testbucket.