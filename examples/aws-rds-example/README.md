# AWS RDS Module Example

## Setup Steps

Install the Terraform modules using `pulumi package add`.

```console
$ yarn install
$ pulumi package add terraform-module terraform-aws-modules/vpc/aws 5.19.0 vpcmod
$ pulumi package add terraform-module terraform-aws-modules/rds/aws 6.10.0 rdsmod
```

## Deploy

To deploy the example run

```console
$ pulumi up
```
