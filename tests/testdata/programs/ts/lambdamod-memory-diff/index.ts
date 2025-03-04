import * as pulumi from "@pulumi/pulumi";
import * as lambda from "@pulumi/lambda";
import * as path from "path";


const config = new pulumi.Config();
const prefix = config.get('prefix') ?? pulumi.getStack();

const testlambda = new lambda.Module("test-lambda", {
    function_name: `${prefix}-testlambda`,
    source_path:  path.join(__dirname, "src/app.ts"),
    artifacts_dir: path.join(__dirname, "builds"),
    runtime:  "nodejs22.x",
    handler: "app.handler",
})


export const lambdaId =  testlambda.lambda_function_arn
