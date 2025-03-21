# AWS ALB Module Example

## Setup Steps

Install the Terraform modules using `pulumi package add`.

```console
$ yarn install
$ pulumi package add terraform-module terraform-aws-modules/vpc/aws 5.19.0 vpcmod
$ pulumi package add terraform-module terraform-aws-modules/alb/aws 9.14.0 albmod
$ pulumi package add terraform-module terraform-aws-modules/s3-bucket/aws 4.6.0 bucketmod
$ pulumi package add terraform-module terraform-aws-modules/lambda/aws 7.20.1 lambdamod
```

## Deploy

To deploy the example run

```console
$ pulumi up
```
