import * as pulumi from "@pulumi/pulumi";
import * as terraformAwsModules from "@pulumi/terraform-aws-modules";


const lambda = new terraformAwsModules.Lambda("test-lambda", {
    function_name: "testLambda",
    source_path: "no/fucking/clue"
})

export const lambdaId =  lambda.lambda_function_arn