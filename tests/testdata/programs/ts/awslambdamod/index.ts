import * as pulumi from "@pulumi/pulumi";
import * as lambda from "@pulumi/lambda";
import * as path from "path";

const testlambda = new lambda.Module("test-lambda", {
    function_name: "testlambda",
    source_path:  path.join(process.env["PWD"], "/src/app.ts"),
    runtime:  "nodejs22.x",
    handler: "app.handler",
})

export const lambdaId =  testlambda.lambda_function_arn