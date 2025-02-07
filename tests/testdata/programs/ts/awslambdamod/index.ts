import * as pulumi from "@pulumi/pulumi";
import * as terraformAwsModules from "@pulumi/terraform-aws-modules";
import * as path from "path";

const testlambda = new terraformAwsModules.Lambda("test-lambda", {
    function_name: "guinstestlambda",
    source_path: path.join(process.env["PWD"], "/src/app.ts"),
    runtime:  "nodejs16.x",
    handler: "app.handler",
})

export const lambdaId =  testlambda.lambda_function_arn