import * as pulumi from "@pulumi/pulumi";
import * as terraformAwsModules from "@pulumi/terraform-aws-modules";



const testlambda = new terraformAwsModules.Lambda("test-lambda", {
    function_name: "guinstestlambda",
    source_path: ("./src/app.ts"),
})

export const lambdaId =  testlambda.lambda_function_arn